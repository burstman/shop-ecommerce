package services

import (
	"log/slog"
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
}
