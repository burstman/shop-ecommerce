package services

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
)

// These tests exercise the three WhatsApp order-status phases
// (confirmed → in-transit → delivered) against an in-memory SQLite DB with a
// fake Cloud API client, so they run offline with plain `go test ./...`.
//
// The seam lives in mescolis_handler.go: newWhatsAppCloudClient builds the
// sender and loadScopedConfigFunc resolves the shop config; both are swapped
// here to avoid the network and the settings table.

type flowRecordedSend struct {
	kind      string // "template" | "confirm" | "urlbtn"
	phone     string
	template  string
	lang      string
	params    []string
	urlSuffix string
}

type flowCloudClient struct {
	sends *[]flowRecordedSend
}

func (c *flowCloudClient) record(kind, phone, template, lang string, params []string, urlSuffix string) error {
	*c.sends = append(*c.sends, flowRecordedSend{
		kind: kind, phone: phone, template: template,
		lang: lang, params: params, urlSuffix: urlSuffix,
	})
	return nil
}

func (c *flowCloudClient) SendTemplate(phone, templateName, langCode string, params []string) error {
	return c.record("template", phone, templateName, langCode, params, "")
}

func (c *flowCloudClient) SendOrderConfirmationTemplate(phone, orderURLSuffix string, bodyParams []string) error {
	return c.record("confirm", phone, "", "", bodyParams, orderURLSuffix)
}

func (c *flowCloudClient) SendTemplateWithURLButton(phone, templateName, langCode, urlSuffix string, bodyParams []string) error {
	return c.record("urlbtn", phone, templateName, langCode, bodyParams, urlSuffix)
}

var flowTestDBOnce sync.Once

func flowTestConfig() *config.Config {
	return &config.Config{
		WhatsApp: config.WhatsAppConfig{
			Enabled:                    true,
			PhoneNumberID:              "999999999999999",
			AccessToken:                "test-access-token",
			TemplateName:               "order_update",
			TemplateLang:               "fr",
			OrderInTransitTemplateName: "shipping",
			OrderDeliveredTemplateName: "delivred_order",
		},
	}
}

func setupWhatsAppFlowTest(t *testing.T) (*config.Config, *[]flowRecordedSend) {
	t.Helper()
	os.Setenv("DB_DRIVER", "sqlite3")
	os.Setenv("DB_NAME", "file::memory:?cache=shared")
	os.Setenv("SUPERKIT_SECRET", "test-secret")

	flowTestDBOnce.Do(func() {
		if err := db.Connect(); err != nil {
			t.Fatalf("db connect: %v", err)
		}
		if err := db.Get().AutoMigrate(&models.Order{}, &models.OrderItem{}, &models.Rating{}); err != nil {
			t.Fatalf("auto migrate: %v", err)
		}
	})

	cfg := flowTestConfig()
	sends := &[]flowRecordedSend{}

	prevClient := newWhatsAppCloudClient
	newWhatsAppCloudClient = func(phoneNumberID, accessToken string) whatsappSender {
		return &flowCloudClient{sends: sends}
	}

	prevCfg := loadScopedConfigFunc
	loadScopedConfigFunc = func(models.Order) *config.Config { return cfg }

	prevSecret := os.Getenv("SUPERKIT_SECRET")
	t.Cleanup(func() {
		newWhatsAppCloudClient = prevClient
		loadScopedConfigFunc = prevCfg
		os.Setenv("SUPERKIT_SECRET", prevSecret)
	})

	return cfg, sends
}

func createFlowOrder(t *testing.T, phone string) models.Order {
	t.Helper()
	order := models.Order{
		FirstName: "Hamed",
		LastName:  "Flissi",
		Phone:     phone,
		Status:    "pending",
		Total:     models.Currency(100),
	}
	if err := db.Get().Create(&order).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}
	return order
}

func reloadOrder(t *testing.T, id uint) models.Order {
	t.Helper()
	var order models.Order
	if err := db.Get().First(&order, id).Error; err != nil {
		t.Fatalf("reload order %d: %v", id, err)
	}
	return order
}

func lastSend(t *testing.T, sends *[]flowRecordedSend) flowRecordedSend {
	t.Helper()
	if len(*sends) == 0 {
		t.Fatal("expected at least one send, got none")
	}
	return (*sends)[len(*sends)-1]
}

func assertRatingURLSuffix(t *testing.T, suffix string, orderID uint) {
	t.Helper()
	want := fmt.Sprintf("?id=%d&t=%s", orderID, OrderTrackingToken(orderID))
	if suffix != want {
		t.Errorf("rating url suffix = %q, want %q", suffix, want)
	}
}

// TestWhatsAppFlowThreePhases walks one order through the full:
// confirmed → in-transit → delivered lifecycle and checks the template, phone
// language, body params and the delivered rating-button token at each step.
func TestWhatsAppFlowThreePhases(t *testing.T) {
	_, sends := setupWhatsAppFlowTest(t)
	order := createFlowOrder(t, "21654116584")

	// Phase 1: confirmation template (fixed Arabic template, dynamic URL).
	SendOrderConfirmation(order, "/orders/24")
	if len(*sends) != 1 {
		t.Fatalf("phase 1: got %d sends, want 1", len(*sends))
	}
	s := lastSend(t, sends)
	if s.kind != "confirm" {
		t.Errorf("phase 1 kind = %q, want confirm", s.kind)
	}
	if s.phone != "21654116584" {
		t.Errorf("phase 1 phone = %q", s.phone)
	}
	if s.urlSuffix != "/orders/24" {
		t.Errorf("phase 1 url = %q", s.urlSuffix)
	}
	if len(s.params) != 3 || s.params[1] != fmt.Sprintf("%d", order.ID) {
		t.Errorf("phase 1 params = %v", s.params)
	}

	// Phase 2: in-transit (shipping) template → stamps in_transit_notified_at.
	if err := db.Get().Model(&order).Updates(map[string]any{
		"mescolis_status":       "in-progress",
		"status":                "shipped",
		"mescolis_driver_name":  "Ali",
		"mescolis_driver_phone": "20123456",
	}).Error; err != nil {
		t.Fatalf("set in-transit: %v", err)
	}
	SendInTransitForOrder(order.ID)

	s = lastSend(t, sends)
	if s.kind != "template" || s.template != "shipping" || s.lang != "ar" {
		t.Fatalf("phase 2 send = %+v", s)
	}
	wantParams := []string{
		fmt.Sprintf("%d", order.ID),
		"Ali",
		"21620123456", // 8-digit local driver phone normalized to +216
	}
	if fmt.Sprint(s.params) != fmt.Sprint(wantParams) {
		t.Errorf("phase 2 params = %v, want %v", s.params, wantParams)
	}
	if got := reloadOrder(t, order.ID); got.InTransitNotifiedAt == nil {
		t.Error("phase 2: in_transit_notified_at not stamped")
	}

	// Backfill idempotency: a second call must not send again.
	SendInTransitForOrder(order.ID)
	if len(*sends) != 2 {
		t.Fatalf("phase 2 double-send: got %d sends, want 2", len(*sends))
	}

	// Phase 3: delivered → rating URL button → stamps delivered_notified_at.
	if err := db.Get().Model(&order).Updates(map[string]any{
		"mescolis_status": "delivered",
		"status":          "completed",
	}).Error; err != nil {
		t.Fatalf("set delivered: %v", err)
	}
	SendDeliveredForOrder(order.ID)

	s = lastSend(t, sends)
	if s.kind != "urlbtn" || s.template != "delivred_order" || s.lang != "ar" {
		t.Fatalf("phase 3 send = %+v", s)
	}
	assertRatingURLSuffix(t, s.urlSuffix, order.ID)
	if len(s.params) != 2 || s.params[1] != fmt.Sprintf("%d", order.ID) {
		t.Errorf("phase 3 params = %v", s.params)
	}
	if got := reloadOrder(t, order.ID); got.DeliveredNotifiedAt == nil {
		t.Error("phase 3: delivered_notified_at not stamped")
	}

	SendDeliveredForOrder(order.ID)
	if len(*sends) != 3 {
		t.Fatalf("phase 3 double-send: got %d sends, want 3", len(*sends))
	}
}

// TestWhatsAppFlowPhoneLanguage checks that a non-Tunisian phone receives the
// French locale (template language) for phases 2 and 3.
func TestWhatsAppFlowPhoneLanguage(t *testing.T) {
	_, sends := setupWhatsAppFlowTest(t)
	order := createFlowOrder(t, "33123456789")

	if err := db.Get().Model(&order).Updates(map[string]any{
		"mescolis_status":       "in-progress",
		"mescolis_driver_name":  "Marie",
		"mescolis_driver_phone": "+33655554444",
	}).Error; err != nil {
		t.Fatalf("set in-transit: %v", err)
	}
	SendInTransitForOrder(order.ID)

	s := lastSend(t, sends)
	if s.template != "shipping" || s.lang != "fr_FR" {
		t.Errorf("phase 2 lang = %q (template %q), want fr_FR/shipping", s.lang, s.template)
	}

	if err := db.Get().Model(&order).Updates(map[string]any{
		"mescolis_status": "delivered",
		"status":          "completed",
	}).Error; err != nil {
		t.Fatalf("set delivered: %v", err)
	}
	SendDeliveredForOrder(order.ID)

	s = lastSend(t, sends)
	if s.template != "delivred_order" || s.lang != "fr_FR" {
		t.Errorf("phase 3 lang = %q (template %q), want fr_FR/delivred_order", s.lang, s.template)
	}
}

// TestWhatsAppFlowBlockedRecipient verifies that a phone marked undeliverable
// (131026 suppression) never receives any of the three templates.
func TestWhatsAppFlowBlockedRecipient(t *testing.T) {
	_, sends := setupWhatsAppFlowTest(t)
	order := createFlowOrder(t, "21654116584")
	order.WhatsappBlocked = true
	if err := db.Get().Model(&order).Update("whatsapp_blocked", true).Error; err != nil {
		t.Fatalf("mark blocked: %v", err)
	}

	SendOrderConfirmation(order, "/orders/24")
	SendInTransitForOrder(order.ID)
	SendDeliveredForOrder(order.ID)

	if len(*sends) != 0 {
		t.Fatalf("blocked recipient: got %d sends, want 0: %+v", len(*sends), *sends)
	}
}

// TestOrderTrackingTokenRoundTrip checks the HMAC token that protects the public
// tracking and rating pages (and is embedded in the delivered URL button).
func TestOrderTrackingTokenRoundTrip(t *testing.T) {
	os.Setenv("SUPERKIT_SECRET", "test-secret")
	defer os.Setenv("SUPERKIT_SECRET", "")

	tok := OrderTrackingToken(42)
	if !ValidOrderTrackingToken(42, tok) {
		t.Fatalf("token %q for order 42 should be valid", tok)
	}
	if ValidOrderTrackingToken(43, tok) {
		t.Error("token for order 42 must not validate for order 43")
	}
	if ValidOrderTrackingToken(42, "") {
		t.Error("empty token must not validate")
	}
	if tok != OrderTrackingToken(42) {
		t.Error("token is not deterministic for the same order ID")
	}

	// The suffix embedded in the phase-3 URL button must validate end-to-end.
	want := fmt.Sprintf("?id=42&t=%s", tok)
	if !strings.HasPrefix(want, fmt.Sprintf("?id=42&t=%s", OrderTrackingToken(42))) {
		t.Error("rating suffix does not embed the valid token")
	}
}

// TestWhatsAppSlangAndPhoneNormalization covers the phone→language rule and the
// 8-digit local number → international normalization used by all send helpers.
func TestWhatsAppSlangAndPhoneNormalization(t *testing.T) {
	if got := WhatsAppLangForPhone("21654116584", "fr"); got != "ar" {
		t.Errorf("216 phone lang = %q, want ar", got)
	}
	if got := WhatsAppLangForPhone("21654116584", "en"); got != "ar" {
		t.Errorf("216 phone lang = %q, want ar", got)
	}
	if got := WhatsAppLangForPhone("33123456789", "fr"); got != "fr" {
		t.Errorf("fr phone lang = %q, want fr", got)
	}
	if got := WhatsAppLangForPhone("33123456789", "en"); got != "en" {
		t.Errorf("fr phone lang = %q, want en", got)
	}

	if got := NormalizeWhatsAppPhone("20123456"); got != "21620123456" {
		t.Errorf("normalize local = %q, want 21620123456", got)
	}
	if got := NormalizeWhatsAppPhone("21620123456"); got != "21620123456" {
		t.Errorf("normalize intl = %q, want 21620123456", got)
	}
	if got := NormalizeWhatsAppPhone("+33123456789"); got != "+33123456789" {
		t.Errorf("normalize + = %q, want left alone", got)
	}
}