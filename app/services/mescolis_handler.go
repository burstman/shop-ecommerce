package services

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
)

// whatsappSender is the subset of the WhatsApp Cloud API used by the order-status
// flow (confirmed → in-transit → delivered). It exists so tests can swap the live
// HTTP client for a fake recorder without touching the network.
type whatsappSender interface {
	SendTemplate(phone, templateName, langCode string, params []string) error
	SendOrderConfirmationTemplate(phone, orderURLSuffix string, bodyParams []string) error
	SendTemplateWithURLButton(phone, templateName, langCode, urlSuffix string, bodyParams []string) error
}

// newWhatsAppCloudClient builds a sender from shop credentials. Tests override it
// (together with loadScopedConfigFunc) to exercise the flow offline.
var newWhatsAppCloudClient = func(phoneNumberID, accessToken string) whatsappSender {
	return NewWhatsAppCloudClient(phoneNumberID, accessToken)
}

// loadScopedConfigFunc resolves the affiliate-scoped config for an order. Tests
// override it to return an in-memory config instead of reading the settings table.
var loadScopedConfigFunc = loadScopedConfig

// HandleMescolisEvent updates an order when Mes Colis sends a status change.
func HandleMescolisEvent(evt MescolisEvent) {
	if evt.Barcode == "" || evt.Status == "" {
		return
	}

	var order models.Order
	if err := db.Get().Where("mescolis_barcode = ?", evt.Barcode).First(&order).Error; err != nil {
		slog.Warn("mescolis: received event for unknown barcode", "barcode", evt.Barcode, "status", evt.Status)
		return
	}

	updates := map[string]any{
		"mescolis_status": evt.Status,
	}

	// Map terminal Mes Colis statuses to our order status.
	switch evt.Status {
	case "in-progress":
		updates["status"] = "shipped"
		updates["mescolis_driver_name"] = evt.DeliverymanName
		updates["mescolis_driver_phone"] = stripNonDigits(evt.DeliverymanPhoneNumber)
	case "delivered", "delivered-and-paid":
		updates["status"] = "completed"
	case "return-sender", "final-return", "cancelled-by-sender":
		updates["status"] = "cancelled"
	}

	if err := db.Get().Model(&order).Updates(updates).Error; err != nil {
		slog.Error("mescolis: failed to update order", "orderID", order.ID, "err", err)
		return
	}
	slog.Info("mescolis: order updated",
		"orderID", order.ID,
		"barcode", evt.Barcode,
		"mescolis_status", evt.Status,
		"order_status", updates["status"],
	)

	// Send WhatsApp notification to the customer.
	switch evt.Status {
	case "in-progress":
		SendPendingInTransitNotifications()
	case "delivered":
		sendWhatsAppDelivered(order)
	case "return-sender", "final-return", "cancelled-by-sender":
		// Status updated to cancelled above; no WhatsApp template for returns.
	}
}

// whatsappLangForPhone picks the template language for a customer's status update.
// Tunisian numbers (+216) get Arabic; everyone else falls back to the configured default.
func WhatsAppLangForPhone(phone, defaultLang string) string {
	if strings.HasPrefix(phone, "216") {
		return "ar"
	}
	return defaultLang
}

// arabicMonthNames maps Go's time.Month to Arabic month names.
var arabicMonthNames = map[time.Month]string{
	time.January:   "جانفي",
	time.February:  "فيفري",
	time.March:     "مارس",
	time.April:     "أفريل",
	time.May:       "ماي",
	time.June:      "جوان",
	time.July:      "جويلية",
	time.August:    "أوت",
	time.September: "سبتمبر",
	time.October:   "أكتوبر",
	time.November:  "نوفمبر",
	time.December:  "ديسمبر",
}

// arabicDate formats a date as Tunisian Arabic, e.g. "1 جانفي، 2024".
func arabicDate(t time.Time) string {
	month := arabicMonthNames[t.Month()]
	if month == "" {
		month = t.Month().String()
	}
	return fmt.Sprintf("%d %s، %d", t.Day(), month, t.Year())
}

// logSendErr logs a failed whatsapp send, but treats ErrRecipientNotOnWhatsApp
// as an expected, quiet skip.
func logSendErr(kind string, order models.Order, phone string, err error) {
	if errors.Is(err, ErrRecipientNotOnWhatsApp) {
		slog.Info("whatsapp: skipped - recipient has no whatsapp",
			"kind", kind,
			"orderID", order.ID,
			"phone", phone,
		)
		return
	}
	slog.Error("whatsapp: failed to send "+kind,
		"orderID", order.ID,
		"phone", phone,
		"err", err,
	)
}

// loadScopedConfig returns the affiliate-scoped config for an order: WhatsApp
// credentials live per-shop (app_config:AFF-xxx), not in the global app_config.
func loadScopedConfig(order models.Order) *config.Config {
	cfg := config.Get()
	if order.AffiliateID != nil {
		var aff models.Affiliate
		if err := db.Get().First(&aff, *order.AffiliateID).Error; err == nil && aff.AffiliateID != "" {
			cfg = config.LoadByAffiliateID(aff.AffiliateID)
		}
	}
	return cfg
}

// normalizeWhatsAppPhone turns an order phone (possibly 8-digit local) into
// international format without "+" (e.g. "21620123456").
func NormalizeWhatsAppPhone(phone string) string {
	if len(phone) == 8 {
		return "216" + phone
	}
	return phone
}

// SendOrderConfirmation sends the phase-1 `order_confirmed_v2` template when an
// order is confirmed by the admin. The template has 3 body vars (name, order
// number, estimated delivery date) and a dynamic URL button pointing at the
// public order page.
func SendOrderConfirmation(order models.Order, orderURL string) {
	cfg := loadScopedConfigFunc(order)
	if !cfg.WhatsApp.Enabled || cfg.WhatsApp.AccessToken == "" || cfg.WhatsApp.PhoneNumberID == "" {
		return
	}
	if order.Phone == "" {
		return
	}
	if order.WhatsappBlocked {
		slog.Info("whatsapp: skipped confirmation - recipient undeliverable",
			"orderID", order.ID,
			"phone", order.Phone,
		)
		return
	}

	phone := NormalizeWhatsAppPhone(order.Phone)
		client := newWhatsAppCloudClient(cfg.WhatsApp.PhoneNumberID, cfg.WhatsApp.AccessToken)

	name := strings.TrimSpace(order.FirstName + " " + order.LastName)
	if name == "" {
		name = "العميل"
	}

	params := []string{
		name,
		fmt.Sprintf("%d", order.ID),
		arabicDate(time.Now().AddDate(0, 0, 2)),
	}

	err := client.SendOrderConfirmationTemplate(phone, orderURL, params)
	if err != nil {
		logSendErr("order confirmation", order, phone, err)
		return
	}
	slog.Info("whatsapp: order confirmation sent",
		"orderID", order.ID,
		"phone", phone,
		"template", "order_confirmed_v2",
	)
}

// tunisNow returns the current time in Africa/Tunis (UTC+1, no DST).
func tunisNow() time.Time {
	loc, err := time.LoadLocation("Africa/Tunis")
	if err != nil {
		return time.Now()
	}
	return time.Now().In(loc)
}

// inTransitWindowOpen reports whether it is at or after 10:00 Tunisia time.
func inTransitWindowOpen() bool {
	return tunisNow().Hour() >= 10
}

// markInTransitNotified stamps an order as notified so it is never sent twice.
func markInTransitNotified(orderID uint) {
	if err := db.Get().Model(&models.Order{}).Where("id = ?", orderID).
		Update("in_transit_notified_at", time.Now().UTC()).Error; err != nil {
		slog.Error("whatsapp: failed to mark order in-transit notified",
			"orderID", orderID, "err", err)
	}
}

// SendPendingInTransitNotifications sends the phase-2 in-transit template to any
// order currently out for delivery that hasn't been notified yet. It only sends
// at/after 10:00 Tunisia time and skips orders already notified or blocked.
func SendPendingInTransitNotifications() {
	if !inTransitWindowOpen() {
		slog.Info("whatsapp: in-transit window not open yet (before 10:00 Tunisia), deferring")
		return
	}

	var orders []models.Order
	if err := db.Get().Where("mescolis_status = ? AND in_transit_notified_at IS NULL",
		"in-progress").Find(&orders).Error; err != nil {
		slog.Error("whatsapp: failed to load pending in-transit orders", "err", err)
		return
	}

	for _, order := range orders {
		sendWhatsAppInTransit(order)
	}
}

// SendInTransitForOrder sends the phase-2 in-transit template for a single order
// immediately, ignoring the 10:00 delivery-window check. Intended for manual
// testing / support use.
func SendInTransitForOrder(orderID uint) {
	var order models.Order
	if err := db.Get().First(&order, orderID).Error; err != nil {
		slog.Error("whatsapp: in-transit send: order not found", "orderID", orderID, "err", err)
		return
	}
	sendWhatsAppInTransit(order)
}

// SendDeliveredForOrder advances orderID to the MesColis "delivered" state and
// sends the phase-3 order_delivered template (rating URL button). Order is KEPT
// so the rating button link stays live.
func SendDeliveredForOrder(orderID uint) {
	var order models.Order
	if err := db.Get().First(&order, orderID).Error; err != nil {
		slog.Error("whatsapp: delivered send: order not found", "orderID", orderID, "err", err)
		return
	}
	sendWhatsAppDelivered(order)
}

// markDeliveredNotified stamps an order as notified post-delivery.
func markDeliveredNotified(orderID uint) {
	if err := db.Get().Model(&models.Order{}).Where("id = ?", orderID).
		Update("delivered_notified_at", time.Now().UTC()).Error; err != nil {
		slog.Error("whatsapp: failed to mark order delivered-notified",
			"orderID", orderID, "err", err)
	}
}

// sendWhatsAppDelivered sends the phase-3 template when MesColis marks a parcel
// "delivered". Body carries the customer name + order number, and a dynamic URL
// button points at the protected /rating page (?id={id}&t={token}). Stamps
// delivered_notified_at so it is sent at most once.
func sendWhatsAppDelivered(order models.Order) {
	if order.DeliveredNotifiedAt != nil {
		return
	}
	cfg := loadScopedConfigFunc(order)
	if !cfg.WhatsApp.Enabled || cfg.WhatsApp.AccessToken == "" || cfg.WhatsApp.PhoneNumberID == "" {
		return
	}
	if cfg.WhatsApp.OrderDeliveredTemplateName == "" {
		slog.Warn("whatsapp: delivered template name not configured", "orderID", order.ID)
		return
	}
	if order.Phone == "" {
		return
	}
	if order.WhatsappBlocked {
		slog.Info("whatsapp: skipped delivered - recipient undeliverable",
			"orderID", order.ID,
			"phone", order.Phone,
		)
		return
	}

	name := strings.TrimSpace(order.FirstName + " " + order.LastName)
	if name == "" {
		name = "العميل"
	}

	phone := NormalizeWhatsAppPhone(order.Phone)

	lang := WhatsAppLangForPhone(phone, cfg.WhatsApp.TemplateLang)
	if lang == "" {
		lang = "fr"
	}
	langMap := map[string]string{
		"fr": "fr_FR",
		"en": "en_US",
		"ar": "ar",
	}
	if full, ok := langMap[lang]; ok {
		lang = full
	}

	ratingSuffix := fmt.Sprintf("?id=%d&t=%s", order.ID, OrderTrackingToken(order.ID))

		client := newWhatsAppCloudClient(cfg.WhatsApp.PhoneNumberID, cfg.WhatsApp.AccessToken)
	err := client.SendTemplateWithURLButton(phone, cfg.WhatsApp.OrderDeliveredTemplateName, lang, ratingSuffix, []string{
		name,
		fmt.Sprintf("%d", order.ID),
	})
	if err != nil {
		logSendErr("delivered update", order, phone, err)
		return
	}
	markDeliveredNotified(order.ID)
	slog.Info("whatsapp: delivered update sent",
		"orderID", order.ID,
		"phone", phone,
		"template", cfg.WhatsApp.OrderDeliveredTemplateName,
	)
}

// sendWhatsAppInTransit sends the phase-2 template for an order marked
// "in-progress" by Mes Colis. The template carries the delivery driver's name
// and phone as plain body variables; WhatsApp linkifies the number if it
// recognizes it. On success it stamps in_transit_notified_at.
func sendWhatsAppInTransit(order models.Order) {
	if order.InTransitNotifiedAt != nil {
		return
	}
	cfg := loadScopedConfigFunc(order)
	if !cfg.WhatsApp.Enabled || cfg.WhatsApp.AccessToken == "" || cfg.WhatsApp.PhoneNumberID == "" {
		return
	}
	if cfg.WhatsApp.OrderInTransitTemplateName == "" {
		slog.Warn("whatsapp: in-transit template name not configured", "orderID", order.ID)
		return
	}
	if order.Phone == "" {
		return
	}
	if order.WhatsappBlocked {
		slog.Info("whatsapp: skipped in-transit - recipient undeliverable",
			"orderID", order.ID,
			"phone", order.Phone,
		)
		return
	}

	phone := NormalizeWhatsAppPhone(order.Phone)

	lang := WhatsAppLangForPhone(phone, cfg.WhatsApp.TemplateLang)
	if lang == "" {
		lang = "fr"
	}
	langMap := map[string]string{
		"fr": "fr_FR",
		"en": "en_US",
		"ar": "ar",
	}
	if full, ok := langMap[lang]; ok {
		lang = full
	}

	driverName := strings.TrimSpace(order.MescolisDriverName)
	if driverName == "" {
		driverName = "عامل التوصيل"
	}
	driverPhone := NormalizeWhatsAppPhone(stripNonDigits(order.MescolisDriverPhone))

		client := newWhatsAppCloudClient(cfg.WhatsApp.PhoneNumberID, cfg.WhatsApp.AccessToken)
	err := client.SendTemplate(phone, cfg.WhatsApp.OrderInTransitTemplateName, lang, []string{
		fmt.Sprintf("%d", order.ID),
		driverName,
		driverPhone,
	})
	if err != nil {
		logSendErr("in-transit update", order, phone, err)
		return
	}
	markInTransitNotified(order.ID)
	slog.Info("whatsapp: in-transit update sent",
		"orderID", order.ID,
		"phone", phone,
		"template", cfg.WhatsApp.OrderInTransitTemplateName,
		"driver", driverName,
		"driver_phone", driverPhone,
	)
}

// stripNonDigits keeps only the numeric characters of a phone string.
func stripNonDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
