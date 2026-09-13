package main

import (
	"fmt"
	"os"
	"strconv"

	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
)

// Usage: DATABASE_URL="postgresql://..." go run cmd/fake-inprogress/main.go [orderID]
// Advances an order to the MesColis "in-progress" state (status=shipped, driver
// info stored) and sends the in_progress template immediately (bypasses the
// 10:00 window). Default orderID is 0, which creates a fresh test order.
func main() {
	if err := db.Connect(); err != nil {
		fmt.Printf("db connect: %v\n", err)
		os.Exit(1)
	}

	var order models.Order

	if len(os.Args) > 1 {
		id, err := strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Printf("invalid order id %q\n", os.Args[1])
			os.Exit(1)
		}
		if err := db.Get().First(&order, id).Error; err != nil {
			fmt.Printf("order %d not found: %v\n", id, err)
			os.Exit(1)
		}
		fmt.Printf("using existing order id=%d (status=%s)\n", order.ID, order.Status)
	} else {
		var aff models.Affiliate
		if err := db.Get().Where("affiliate_id = ?", "AFF-001").First(&aff).Error; err != nil {
			fmt.Printf("affiliate AFF-001 not found: %v\n", err)
			os.Exit(1)
		}
		order = models.Order{
			FirstName: "Hamed",
			LastName:  "Flissi",
			Phone:     "21654116584",
			Total:     100,
			Status:    "shipped",
			IsTest:    true,
			AffiliateID: &aff.ID,
		}
		fmt.Println("created fresh test order")
	}

	updates := map[string]any{
		"status":                "shipped",
		"mescolis_status":       "in-progress",
		"mescolis_driver_name":  "عامل التوصيل التجريبي",
		"mescolis_driver_phone": "21620222222",
	}
	if err := db.Get().Model(&order).Updates(updates).Error; err != nil {
		fmt.Printf("update order: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("order id=%d now in-progress\n", order.ID)

	services.SendInTransitForOrder(order.ID)

	trackingURL := fmt.Sprintf("https://shop-ecommerce-9kak.onrender.com/tracking?id=%d&t=%s",
		order.ID, services.OrderTrackingToken(order.ID))
	fmt.Printf("\nTRACKING URL: %s\n", trackingURL)
}