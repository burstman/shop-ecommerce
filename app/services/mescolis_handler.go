package services

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
)

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
	sendWhatsAppStatusUpdate(order, evt.Status)
}

// whatsappLangForPhone picks the template language for a customer's status update.
// Tunisian numbers (+216) get Arabic; everyone else falls back to the configured default.
func whatsappLangForPhone(phone, defaultLang string) string {
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

// sendWhatsAppStatusUpdate sends a WhatsApp template message with the parcel status.
func sendWhatsAppStatusUpdate(order models.Order, mescolisStatus string) {
	// Load the affiliate-scoped config: WhatsApp credentials live per-shop
	// (app_config:AFF-xxx), not in the global app_config.
	cfg := config.Get()
	if order.AffiliateID != nil {
		var aff models.Affiliate
		if err := db.Get().First(&aff, *order.AffiliateID).Error; err == nil && aff.AffiliateID != "" {
			cfg = config.LoadByAffiliateID(aff.AffiliateID)
		}
	}
	if !cfg.WhatsApp.Enabled || cfg.WhatsApp.AccessToken == "" || cfg.WhatsApp.PhoneNumberID == "" || cfg.WhatsApp.TemplateName == "" {
		return
	}
	if order.Phone == "" {
		return
	}

	// Format phone: 8-digit local → "216XXXXXXXX" (Tunisia country code, no +)
	phone := order.Phone
	if len(phone) == 8 {
		phone = "216" + phone
	}

	lang := whatsappLangForPhone(phone, cfg.WhatsApp.TemplateLang)
	if lang == "" {
		lang = "fr"
	}
	// Map short codes to full Meta locale codes if needed
	langMap := map[string]string{
		"fr": "fr_FR",
		"en": "en_US",
		"ar": "ar",
	}
	if full, ok := langMap[lang]; ok {
		lang = full
	}

	trackingURL := fmt.Sprintf("https://mescolis.tn/suivi/%s", order.MescolisBarcode)

	client := NewWhatsAppCloudClient(cfg.WhatsApp.PhoneNumberID, cfg.WhatsApp.AccessToken)

	// Tunisian numbers get the Arabic tracking template; everyone else uses the
	// configured template + language.
	if strings.HasPrefix(phone, "216") {
		err := client.SendTemplate(phone, "order_ar_tracking", "ar", []string{
			fmt.Sprintf("%d", order.ID),
			arabicDate(time.Now().AddDate(0, 0, 2)),
		})
		if err != nil {
			slog.Error("whatsapp: failed to send arabic status update",
				"orderID", order.ID,
				"phone", phone,
				"err", err,
			)
			return
		}
		slog.Info("whatsapp: arabic status update sent",
			"orderID", order.ID,
			"phone", phone,
			"template", "order_ar_tracking",
		)
		return
	}

	err := client.SendTemplate(phone, cfg.WhatsApp.TemplateName, lang, []string{
		fmt.Sprintf("%d", order.ID),
		mescolisStatus,
		trackingURL,
	})
	if err != nil {
		slog.Error("whatsapp: failed to send status update",
			"orderID", order.ID,
			"phone", phone,
			"err", err,
		)
		return
	}
	slog.Info("whatsapp: status update sent",
		"orderID", order.ID,
		"phone", phone,
		"status", mescolisStatus,
	)
}
