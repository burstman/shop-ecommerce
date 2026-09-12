package main

import (
	"fmt"
	"log"
	"os"

	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
)

// Usage: DATABASE_URL="postgresql://..." go run cmd/fake-inprogress/main.go
// Creates a test order flagged in-progress and sends the in_progress template.
func main() {
	if err := db.Connect(); err != nil {
		log.Fatalf("db connect: %v", err)
	}

	var aff models.Affiliate
	if err := db.Get().Where("affiliate_id = ?", "AFF-001").First(&aff).Error; err != nil {
		log.Fatalf("affiliate AFF-001 not found: %v", err)
	}

	order := models.Order{
		FirstName:          "Test",
		LastName:           "Driver",
		Phone:              "21654116584",
		Total:              100,
		Status:             "shipped",
		IsTest:             true,
		AffiliateID:        &aff.ID,
		MescolisBarcode:    "TEST-INPROGRESS",
		MescolisStatus:     "in-progress",
		MescolisDriverName: "عامل التوصيل التجريبي",
		MescolisDriverPhone: "21620222222",
	}

	if err := db.Get().Create(&order).Error; err != nil {
		log.Fatalf("create order: %v", err)
	}
	fmt.Printf("created test order id=%d\n", order.ID)

	services.SendInTransitForOrder(order.ID)

	if err := db.Get().Unscoped().Delete(&order).Error; err != nil {
		fmt.Printf("cleanup failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("cleaned up test order")
}