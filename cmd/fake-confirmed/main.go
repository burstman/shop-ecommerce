package main

import (
	"fmt"
	"log"

	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
)

// Usage: DATABASE_URL="postgresql://..." go run cmd/fake-confirmed/main.go
// Creates a confirmed order for phone 21654116584, sends the order_confirmed_v2
// template, and prints the tracking URL. The order is KEPT so the tracking page
// shows the confirmed status.
func main() {
	if err := db.Connect(); err != nil {
		log.Fatalf("db connect: %v", err)
	}

	var aff models.Affiliate
	if err := db.Get().Where("affiliate_id = ?", "AFF-001").First(&aff).Error; err != nil {
		log.Fatalf("affiliate AFF-001 not found: %v", err)
	}

	order := models.Order{
		FirstName:   "Hamed",
		LastName:    "Flissi",
		Phone:       "21654116584",
		Total:       100,
		Status:      "confirmed",
		IsTest:      true,
		AffiliateID: &aff.ID,
	}

	if err := db.Get().Create(&order).Error; err != nil {
		log.Fatalf("create order: %v", err)
	}

	orderURL := fmt.Sprintf("?id=%d&t=%s", order.ID, services.OrderTrackingToken(order.ID))
	fmt.Printf("created confirmed order id=%d\n", order.ID)

	services.SendOrderConfirmation(order, orderURL)

	trackingURL := fmt.Sprintf("https://shop-ecommerce-9kak.onrender.com/tracking?id=%d&t=%s",
		order.ID, services.OrderTrackingToken(order.ID))
	fmt.Printf("\nTRACKING URL: %s\n", trackingURL)
	fmt.Println("(order kept; delete it manually when done testing)")
}