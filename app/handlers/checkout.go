package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/helpers"
	"shopTemplate/app/models"
	"shopTemplate/app/views/checkout"
	"strconv"
	"time"

	"github.com/anthdm/superkit/event"
	"github.com/anthdm/superkit/kit"
	"github.com/anthdm/superkit/validate"
)

func getAffiliateID(ctx context.Context) *uint {
	affiliate := config.AffiliateFromContext(ctx)
	if affiliate == nil {
		return nil
	}
	return &affiliate.ID
}

func HandleCheckoutIndex(kit *kit.Kit) error {
	cart := helpers.GetCart(kit)
	cfg := config.FromContext(kit.Request.Context())
	return RenderWithLayout(kit, checkout.Index(cart, make(map[string]string), make(validate.Errors), cfg))
}

func HandleCheckoutSuccess(kit *kit.Kit) error {
	cfg := config.FromContext(kit.Request.Context())
	totalStr := kit.Request.URL.Query().Get("total")
	total, err := strconv.ParseFloat(totalStr, 64)
	if err != nil {
		total = 0
	}
	return RenderWithLayout(kit, checkout.Success(cfg, total))
}

func HandleCheckoutCreate(kit *kit.Kit) error {
	errors := make(validate.Errors)
	phone := kit.Request.FormValue("phone")
	cfg := config.FromContext(kit.Request.Context())

	// Server-side validation for exactly 8 digits
	matched, err := regexp.MatchString(`^[0-9]{8}$`, phone)
	if err != nil || !matched {
		errors.Add("phone", "Phone number must be exactly 8 digits")
	}

	if len(errors) > 0 {
		cart := helpers.GetCart(kit)
		values := map[string]string{
			"firstName": kit.Request.FormValue("firstName"),
			"lastName":  kit.Request.FormValue("lastName"),
			"email":     kit.Request.FormValue("email"),
			"address":   kit.Request.FormValue("address"),
			"city":      kit.Request.FormValue("city"),
			"phone":     phone,
		}
		return RenderWithLayout(kit, checkout.Index(cart, values, errors, cfg))
	}

	// 1. Create the Order
	cart := helpers.GetCart(kit)
	total := calculateTotal(cart, cfg.Site.Bundles)
	// Calculate 1% commission with proper rounding
	commission := models.Currency(math.Round(float64(total) / 100.0))

	// Detect test order (phone = 00000000)
	isTest := phone == "00000000"
	if isTest {
		slog.Info("test order detected during checkout", "phone", phone)
		commission = 0
	}

	affiliateID := getAffiliateID(kit.Request.Context())

	// Check for existing abandoned order
	sess := kit.GetSession("session")
	abandonedID, _ := sess.Values["abandoned_order_id"].(uint)

	order := models.Order{
		FirstName:          kit.Request.FormValue("firstName"),
		LastName:           kit.Request.FormValue("lastName"),
		Email:              kit.Request.FormValue("email"),
		Address:            kit.Request.FormValue("address"),
		City:               kit.Request.FormValue("city"),
		Phone:              phone,
		Total:              total,
		PlatformCommission: commission,
		CommissionStatus:   "pending",
		Status: func() string {
			if isTest {
				return "test"
			}
			return "pending"
		}(),
		IsTest:      isTest,
		AffiliateID: affiliateID,
	}

	if abandonedID != 0 {
		var existing models.Order
		if err := db.Get().First(&existing, abandonedID).Error; err == nil && (existing.Status == "abandoned" || existing.Status == "test") {
			order.ID = existing.ID
			order.CreatedAt = existing.CreatedAt
			if err := db.Get().Save(&order).Error; err != nil {
				return err
			}
			// Delete existing items to recreate them with final state
			db.Get().Where("order_id = ?", order.ID).Delete(&models.OrderItem{})
		} else {
			if err := db.Get().Create(&order).Error; err != nil {
				return err
			}
		}
	} else {
		if err := db.Get().Create(&order).Error; err != nil {
			return err
		}
	}

	// Clear abandoned ID from session
	delete(sess.Values, "abandoned_order_id")
	sess.Save(kit.Request, kit.Response)

	// 2. Create Order Items
	for _, item := range cart.Items {
		itemTotal := helpers.CalculateItemPrice(item, cfg.Site.Bundles)
		// Use proper rounding to avoid precision loss on division
		unitPrice := models.Currency(math.Round(float64(itemTotal) / float64(item.Quantity)))

		orderItem := models.OrderItem{
			OrderID:      order.ID,
			ProductID:    item.Product.ID,
			ProductName:  item.Product.Name,
			ProductImage: item.Product.Image,
			Quantity:     item.Quantity,
			Price:        unitPrice,
		}
		if err := db.Get().Create(&orderItem).Error; err != nil {
			slog.Error("failed to create order item", "err", err, "orderID", order.ID)
		}

		// Optional: Deduct stock here
		// db.Get().Model(&item.Product).Update("stock", item.Product.Stock - item.Quantity)
	}

	// 3. Clear the cart
	sess = kit.GetSession("session")
	sess.Values["cart"] = &models.Cart{Items: make(map[uint]*models.CartItem)}
	sess.Save(kit.Request, kit.Response)

	// Emit events.
	if isTest {
		lastTest, _ := sess.Values["last_test_notified_at"].(int64)
		if time.Now().Unix()-lastTest >= 60 {
			slog.Info("triggering test notification for 8 zeros")
			event.Emit("order.placed", order)
			sess.Values["last_test_notified_at"] = time.Now().Unix()
			sess.Save(kit.Request, kit.Response)
		} else {
			slog.Info("test notification rate limited, skipping")
		}
	} else {
		event.Emit("order.placed", order)
	}

	// Redirect to the success page with total for tracking
	kit.Response.Header().Set("HX-Redirect", fmt.Sprintf("/checkout/success?total=%.2f", order.Total.ToFloat()))
	return nil
}

func HandleCheckoutAbandoned(kit *kit.Kit) error {
	phone := kit.Request.FormValue("phone")
	if len(phone) != 8 {
		return nil
	}

	cart := helpers.GetCart(kit)
	if len(cart.Items) == 0 {
		return nil
	}

	cfg := config.FromContext(kit.Request.Context())
	total := calculateTotal(cart, cfg.Site.Bundles)
	commission := models.Currency(math.Round(float64(total) / 100.0))

	// Detect test order (phone = 00000000)
	isTest := phone == "00000000"
	if isTest {
		slog.Info("test order detected during abandoned checkout processing", "phone", phone)
		commission = 0
	}

	status := "abandoned"
	if isTest {
		status = "test"
	}

	sess := kit.GetSession("session")
	abandonedID, _ := sess.Values["abandoned_order_id"].(uint)

	order := models.Order{
		FirstName:          kit.Request.FormValue("firstName"),
		LastName:           kit.Request.FormValue("lastName"),
		Email:              kit.Request.FormValue("email"),
		Address:            kit.Request.FormValue("address"),
		City:               kit.Request.FormValue("city"),
		Phone:              phone,
		Total:              total,
		PlatformCommission: commission,
		CommissionStatus:   "pending",
		Status:             status,
		IsTest:             isTest,
		AffiliateID:        getAffiliateID(kit.Request.Context()),
	}

	isNewAbandoned := false

	if abandonedID != 0 {
		var existing models.Order
		if err := db.Get().First(&existing, abandonedID).Error; err == nil && (existing.Status == "abandoned" || existing.Status == "test") {
			order.ID = existing.ID
			order.CreatedAt = existing.CreatedAt
			if err := db.Get().Save(&order).Error; err != nil {
				return err
			}
		} else {
			if err := db.Get().Create(&order).Error; err != nil {
				return err
			}
			sess.Values["abandoned_order_id"] = order.ID
			sess.Save(kit.Request, kit.Response)
			isNewAbandoned = true
		}
	} else {
		if err := db.Get().Create(&order).Error; err != nil {
			return err
		}
		sess.Values["abandoned_order_id"] = order.ID
		sess.Save(kit.Request, kit.Response)
		isNewAbandoned = true
	}

	// Sync order items
	db.Get().Where("order_id = ?", order.ID).Delete(&models.OrderItem{})
	for _, item := range cart.Items {
		itemTotal := helpers.CalculateItemPrice(item, cfg.Site.Bundles)
		unitPrice := models.Currency(math.Round(float64(itemTotal) / float64(item.Quantity)))
		orderItem := models.OrderItem{
			OrderID:      order.ID,
			ProductID:    item.Product.ID,
			ProductName:  item.Product.Name,
			ProductImage: item.Product.Image,
			Quantity:     item.Quantity,
			Price:        unitPrice,
		}
		db.Get().Create(&orderItem)
	}

	// Only emit event on first creation, or if it's a test trigger with rate limiting
	shouldNotify := isNewAbandoned
	if isTest {
		lastTest, _ := sess.Values["last_test_notified_at"].(int64)
		if time.Now().Unix()-lastTest >= 60 {
			shouldNotify = true
			sess.Values["last_test_notified_at"] = time.Now().Unix()
			sess.Save(kit.Request, kit.Response)
			slog.Info("triggering test abandoned notification for 8 zeros")
		}
	}

	if shouldNotify {
		event.Emit("order.abandoned", order)
	}

	return nil
}

func calculateTotal(cart *models.Cart, bundles []models.Bundle) models.Currency {
	return helpers.CalculateCartTotal(cart, bundles)
}
