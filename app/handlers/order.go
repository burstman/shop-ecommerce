package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
	viewerrors "shopTemplate/app/views/errors"
	"shopTemplate/app/views/orders"
	"strconv"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func HandleAdminOrdersIndex(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	pageStr := kit.Request.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	perPage := 10

	var total int64
	if err := db.Get().Model(&models.Order{}).Count(&total).Error; err != nil {
		return err
	}
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	offset := (page - 1) * perPage

	var ordersList []models.Order
	if err := db.Get().Order("created_at desc").Limit(perPage).Offset(offset).Find(&ordersList).Error; err != nil {
		return err
	}

	cfg := config.FromContext(kit.Request.Context())
	activePath := "/admin/orders"
	sidebar := config.GetAdminSidebarGroups()
	content := orders.Index(ordersList, page, totalPages, cfg)
	return RenderAdminWithLayout(kit, sidebar, activePath, content)
}

func HandleAdminOrderShow(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return kit.Render(viewerrors.Error500())
	}

	var order models.Order
	if err := db.Get().Preload("Items.Product").First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kit.Render(viewerrors.Error404())
		}
		return err
	}

	activePath := "/admin/orders"
	sidebar := config.GetAdminSidebarGroups()
	content := orders.Show(order)
	return RenderAdminWithLayout(kit, sidebar, activePath, content)
}

func HandleAdminOrderUpdateStatus(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return err
	}
	newStatus := kit.Request.FormValue("status")

	var order models.Order
	if err := db.Get().First(&order, id).Error; err != nil {
		return err
	}

	// Define allowed transitions
	allowed := false
	switch order.Status {
	case "pending":
		if newStatus == "confirmed" || newStatus == "shipped" || newStatus == "cancelled" {
			allowed = true
		}
	case "confirmed":
		if newStatus == "shipped" || newStatus == "pending" || newStatus == "cancelled" {
			allowed = true
		}
	case "shipped":
		if newStatus == "completed" || newStatus == "pending" {
			allowed = true
		}
	case "abandoned":
		if newStatus == "pending" || newStatus == "cancelled" {
			allowed = true
		}
	}

	if !allowed {
		return fmt.Errorf("invalid status transition from %s to %s", order.Status, newStatus)
	}

	if err := db.Get().Model(&order).Update("status", newStatus).Error; err != nil {
		return err
	}

	// Create the Mes Colis Express parcel once the order is confirmed with the client.
	if newStatus == "confirmed" && order.MescolisBarcode == "" {
		cfg := config.FromContext(kit.Request.Context())
		syncMescolisParcel(order, cfg)
	}

	return kit.Redirect(http.StatusSeeOther, fmt.Sprintf("/admin/orders/%d", id))
}

func HandleAdminOrderDeleteConfirm(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return kit.Render(viewerrors.Error500())
	}

	var order models.Order
	if err := db.Get().First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kit.Render(viewerrors.Error404())
		}
		return err
	}

	return kit.Render(orders.DeleteModal(order))
}

func HandleAdminOrderCancelConfirm(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return kit.Render(viewerrors.Error500())
	}

	var order models.Order
	if err := db.Get().First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kit.Render(viewerrors.Error404())
		}
		return err
	}

	return kit.Render(orders.CancelModal(order))
}

func syncMescolisParcel(order models.Order, cfg *config.Config) {
	if order.IsTest || !cfg.Mescolis.Enabled || cfg.Mescolis.APIKey == "" {
		return
	}

	var items []models.OrderItem
	if err := db.Get().Where("order_id = ?", order.ID).Find(&items).Error; err != nil {
		slog.Error("failed to load order items for mescolis", "err", err, "orderID", order.ID)
		return
	}

	productName := ""
	for i, item := range items {
		if i > 0 {
			productName += ", "
		}
		productName += fmt.Sprintf("%s x%d", item.ProductName, item.Quantity)
	}
	if productName == "" {
		productName = fmt.Sprintf("Order #%d", order.ID)
	}

	client := services.NewMescolisClient(cfg.Mescolis.APIKey, cfg.Mescolis.AllowSubAccount, cfg.Mescolis.AccountCode)
	resp, err := client.CreateParcel(services.CreateParcelRequest{
		ProductName:  productName,
		ClientName:   fmt.Sprintf("%s %s", order.FirstName, order.LastName),
		Address:      order.Address,
		Gouvernerate: order.Governorate,
		City:         order.City,
		Location:     order.Location,
		Tel1:         order.Phone,
		Price:        order.Total.ToFloat(),
		Note:         fmt.Sprintf("Order #%d", order.ID),
	})
	if err != nil {
		slog.Error("failed to create mescolis parcel", "err", err, "orderID", order.ID)
		return
	}
	if resp != nil && resp.Barcode != "" {
		order.MescolisBarcode = resp.Barcode
		order.MescolisStatus = "pending"
		if err := db.Get().Model(&order).Updates(map[string]any{
			"mescolis_barcode": resp.Barcode,
			"mescolis_status":  "pending",
		}).Error; err != nil {
			slog.Error("failed to save mescolis barcode", "err", err, "orderID", order.ID)
		}
		slog.Info("mescolis parcel created", "orderID", order.ID, "barcode", resp.Barcode)
	}
}

func HandleAdminOrderDelete(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)

	if err != nil {
		return kit.Render(viewerrors.Error500())
	}
	if err := db.Get().Delete(&models.Order{}, id).Error; err != nil {
		return err
	}

	if kit.Request.Header.Get("HX-Request") == "true" {
		return nil
	}

	return kit.Redirect(http.StatusSeeOther, "/admin/orders")
}
