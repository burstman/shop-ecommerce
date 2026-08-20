package services

import (
	"fmt"
	"log/slog"
	"shopTemplate/app/config"
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

	// Send WhatsApp notification to the customer.
	sendWhatsAppStatusUpdate(order, evt.Status)
}

// sendWhatsAppStatusUpdate sends a WhatsApp template message with the parcel status.
func sendWhatsAppStatusUpdate(order models.Order, mescolisStatus string) {
	cfg := config.Get()
	if !cfg.WhatsApp.Enabled || cfg.WhatsApp.APIKey == "" || cfg.WhatsApp.TemplateName == "" {
		return
	}
	if order.Phone == "" {
		return
	}

	// Format phone: 8-digit local → "216XXXXXXXX" (Tunisia country code, no +)
	phone := order.Phone
	if len(phone) == 8 {
		phone = "216" + phone
	}

	lang := cfg.WhatsApp.TemplateLang
	if lang == "" {
		lang = "fr"
	}

	trackingURL := fmt.Sprintf("https://mescolis.tn/suivi/%s", order.MescolisBarcode)

	client := NewD360Client(cfg.WhatsApp.APIKey)
	err := client.SendTemplate(phone, cfg.WhatsApp.TemplateName, lang, []string{
		fmt.Sprintf("%d", order.ID),
		mescolisStatus,
		trackingURL,
	})
	if err != nil {
		slog.Error("whatsapp: failed to send status update",
			"orderID", order.ID,
			"phone", phone,
			"err", err,
		)
		return
	}
	slog.Info("whatsapp: status update sent",
		"orderID", order.ID,
		"phone", phone,
		"status", mescolisStatus,
	)
}
