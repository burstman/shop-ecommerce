package main

import (
	"fmt"
	"os"
	"strconv"
	"log"

	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
)

// Usage: DATABASE_URL="postgresql://..." go run cmd/fake-confirmed/main.go [orderID]
// Advances an order to "confirmed" and sends the order_confirmed_v2 template.
// Default orderID is 0, which creates a fresh REAL (is_test=false) order so the
// tracking page link works. Pass an existing orderID to reuse it. The order is
// KEPT so the tracking page shows the confirmed status.
func main() {
	if err := db.Connect(); err != nil {
		log.Fatalf("db connect: %v", err)
	}

	var order models.Order

	if len(os.Args) > 1 {
		id, err := strconv.Atoi(os.Args[1])
		if err != nil {
			log.Fatalf("invalid order id %q", os.Args[1])
		}
		if err := db.Get().First(&order, id).Error; err != nil {
			log.Fatalf("order %d not found: %v", id, err)
		}
		fmt.Printf("using existing order id=%d (status=%s)\n", order.ID, order.Status)
	} else {
		var aff models.Affiliate
		if err := db.Get().Where("affiliate_id = ?", "AFF-001").First(&aff).Error; err != nil {
			log.Fatalf("affiliate AFF-001 not found: %v", err)
		}

		order = models.Order{
			FirstName:   "Hamed",
			LastName:    "Flissi",
			Phone:       "21654116584",
			Total:       100,
			Status:      "pending",
			IsTest:      false,
			AffiliateID: &aff.ID,
		}
		if err := db.Get().Create(&order).Error; err != nil {
			log.Fatalf("create order: %v", err)
		}
		fmt.Printf("created real order id=%d\n", order.ID)
	}

	if err := db.Get().Model(&order).Updates(map[string]any{"status": "confirmed"}).Error; err != nil {
		log.Fatalf("update order: %v", err)
	}
	fmt.Printf("order id=%d now confirmed\n", order.ID)

	orderURL := fmt.Sprintf("?id=%d&t=%s", order.ID, services.OrderTrackingToken(order.ID))

	services.SendOrderConfirmation(order, orderURL)

	trackingURL := fmt.Sprintf("https://shop-ecommerce-9kak.onrender.com/tracking?id=%d&t=%s",
		order.ID, services.OrderTrackingToken(order.ID))
	fmt.Printf("\nTRACKING URL: %s\n", trackingURL)
	fmt.Println("(order kept; delete it manually when done testing)")
}