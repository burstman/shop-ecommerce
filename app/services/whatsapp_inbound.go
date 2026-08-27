package services

import (
	"fmt"
	"log/slog"

	"shopTemplate/app/config"
)

// HandleWhatsAppInboundMessage processes a message received from a customer
// on WhatsApp. Currently it forwards customer messages to the shop admin's
// Telegram chat (if configured) so the shop owner can respond manually.
func HandleWhatsAppInboundMessage(phoneNumberID, from, msgType, text string) {
	cfg := config.Get()

	token := cfg.Notification.TelegramBotToken
	chatID := cfg.Notification.TelegramChatID

	if token == "" || chatID == "" {
		slog.Info("whatsapp: inbound message received, no Telegram forwarding configured",
			"from", from, "type", msgType,
		)
		return
	}

	notifier := NewTelegramNotifier()
	msg := fmt.Sprintf(
		"📲 *WhatsApp Message from %s*\n\n"+
			"*Type:* %s\n"+
			"*Message:* %s\n\n"+
			"Reply to this customer by messaging them on WhatsApp.",
		from, msgType, text,
	)
	if err := notifier.sendMessage(token, chatID, msg); err != nil {
		slog.Error("whatsapp: failed to forward inbound message to Telegram",
			"from", from, "err", err,
		)
		return
	}
	slog.Info("whatsapp: inbound message forwarded to Telegram admin", "from", from)
}