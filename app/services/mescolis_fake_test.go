package services

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"shopTemplate/app/db"
	"shopTemplate/app/models"
)

// These tests simulate Mes Colis status events against a real database and send
// the matching WhatsApp templates. They are skipped unless TEST_DATABASE_URL is
// set, so normal `go test ./...` runs stay offline.
//
// Usage:
//
//	TEST_DATABASE_URL="postgresql://..." go test ./app/services/ -run TestFakeMescolis -v
//
// Each test keeps the created order in the database (so the public tracking page
// can show the new status) and prints its tracking URL at the end.

var testDBOnce sync.Once

func setupFakeTestDB(t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB-backed fake event test")
	}
	testDBOnce.Do(func() {
		os.Setenv("DATABASE_URL", os.Getenv("TEST_DATABASE_URL"))
		os.Setenv("DB_DRIVER", "postgres")
		if err := db.Connect(); err != nil {
			t.Fatalf("db connect: %v", err)
		}
	})
}

func newFakeTestOrder(t *testing.T) models.Order {
	t.Helper()
	var aff models.Affiliate
	if err := db.Get().Where("affiliate_id = ?", "AFF-001").First(&aff).Error; err != nil {
		t.Fatalf("affiliate AFF-001 not found: %v", err)
	}

	order := models.Order{
		FirstName:           "Hamed",
		LastName:            "Flissi",
		Phone:               "21654116584",
		Total:               100,
		Status:              "pending",
		IsTest:              true,
		AffiliateID:         &aff.ID,
		MescolisBarcode:     fmt.Sprintf("TEST-%d", time.Now().UnixNano()),
		MescolisStatus:      "",
		MescolisDriverName:  "",
		MescolisDriverPhone: "",
	}
	if err := db.Get().Create(&order).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}
	t.Cleanup(func() {
		db.Get().Unscoped().Delete(&models.Order{}, order.ID)
	})
	return order
}

func fakeTrackingURL(t *testing.T, orderID uint) string {
	t.Helper()
	return fmt.Sprintf("https://shop-ecommerce-9kak.onrender.com/tracking?id=%d&t=%s",
		orderID, OrderTrackingToken(orderID))
}

// TestFakeMescolisInProgressEvent fakes the Mes Colis "in-progress" event:
// marks the order shipped, stores driver info, and sends the in_progress
// template immediately (bypassing the 10:00 window).
func TestFakeMescolisInProgressEvent(t *testing.T) {
	setupFakeTestDB(t)
	order := newFakeTestOrder(t)

	updates := map[string]any{
		"status":                "shipped",
		"mescolis_status":       "in-progress",
		"mescolis_driver_name":  "عامل التوصيل التجريبي",
		"mescolis_driver_phone": "21620222222",
	}
	if err := db.Get().Model(&order).Updates(updates).Error; err != nil {
		t.Fatalf("update order: %v", err)
	}
	t.Logf("fake in-progress: order %d status=shipped", order.ID)

	sendWhatsAppInTransit(order)

	var check models.Order
	db.Get().First(&check, order.ID)
	t.Logf("in_transit_notified_at=%v", check.InTransitNotifiedAt)
	t.Logf("tracking page: %s", fakeTrackingURL(t, order.ID))
}

// TestFakeMescolisDeliveredEvent fakes the Mes Colis "delivered" event: marks
// the order completed and sends the delivered template with the rating link.
func TestFakeMescolisDeliveredEvent(t *testing.T) {
	setupFakeTestDB(t)
	order := newFakeTestOrder(t)

	updates := map[string]any{
		"status":          "completed",
		"mescolis_status": "delivered",
	}
	if err := db.Get().Model(&order).Updates(updates).Error; err != nil {
		t.Fatalf("update order: %v", err)
	}
	t.Logf("fake delivered: order %d status=completed", order.ID)

	sendWhatsAppDelivered(order)

	var check models.Order
	db.Get().First(&check, order.ID)
	t.Logf("delivered_notified_at=%v", check.DeliveredNotifiedAt)
	if check.DeliveredNotifiedAt == nil {
		t.Log("NOTE: delivered message was NOT sent (check that the delivered template name and rating URL are configured in admin → WhatsApp)")
	}
	t.Logf("tracking page: %s", fakeTrackingURL(t, order.ID))
}
