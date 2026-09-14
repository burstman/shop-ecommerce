package main

import (
	"fmt"
	"os"
	"strconv"

	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
)

// Usage: DATABASE_URL="postgresql://..." go run cmd/fake-delivered/main.go [orderID]
// Advances an order to the MesColis "delivered" state (status=delivered,
// mescolis_status=delivered) and sends the phase-3 order_delivered template with a
// dynamic URL button pointing at the per-order /rating page. The order is KEPT so
// the rating link stays live. Default orderID is 0, which creates a fresh test
// order. Pass an existing confirmed/in-progress orderID (e.g. 22) to advance a
// real, still-visible order all the way: the rating button becomes clickable.
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
		fresh := models.Order{
			FirstName:   "Hamed",
			LastName:    "Flissi",
			Phone:       "21654116584",
			Total:       100,
			Status:      "shipped",
			IsTest:      true,
			AffiliateID: &aff.ID,
		}
		if err := db.Get().Create(&fresh).Error; err != nil {
			fmt.Printf("create order: %v\n", err)
			os.Exit(1)
		}
		order = fresh
		fmt.Printf("created fresh test order id=%d\n", order.ID)
	}

	updates := map[string]any{
		"status":          "delivered",
		"mescolis_status": "delivered",
	}
	if err := db.Get().Model(&order).Updates(updates).Error; err != nil {
		fmt.Printf("update order: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("order id=%d now delivered\n", order.ID)

	services.SendDeliveredForOrder(order.ID)

	fmt.Printf("\nRATING URL: https://shop-ecommerce-9kak.onrender.com/rating?id=%d&t=%s\n",
		order.ID, services.OrderTrackingToken(order.ID))
	fmt.Println("(order kept; rating page resolves; delete it manually later)")
}
