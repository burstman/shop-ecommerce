package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
)

// TelegramNotifier implements the OrderNotifier interface using Telegram Bot API.
type TelegramNotifier struct{}

// NewTelegramNotifier creates a new instance.
func NewTelegramNotifier() *TelegramNotifier {
	return &TelegramNotifier{}
}

func (t *TelegramNotifier) Name() string {
	return "Telegram Bot"
}

// Send sends the order notification to the configured Telegram chat.
func (t *TelegramNotifier) Send(order models.Order) error {
	cfg := config.Get()
	token := cfg.Notification.TelegramBotToken
	chatID := cfg.Notification.TelegramChatID

	// Skip if not configured
	if token == "" || chatID == "" {
		if order.Phone == "00000000" {
			slog.Warn("Telegram test trigger detected but Bot Token or Chat ID is missing in configuration")
			var aff models.Affiliate
			if err := db.Get().First(&aff).Error; err == nil {
				ReportWarningAffiliate(&aff, "Telegram test trigger detected but Bot Token or Chat ID is missing")
			}
		}
		return nil
	}

	if order.Phone == "00000000" {
		slog.Info("Telegram test trigger activated", "chatID", chatID)
		return t.SendTest(token, chatID)
	}

	text := fmt.Sprintf(
		"🔔 *New Order Received #%d*\n\n"+
			"*Customer:* %s %s (%s)\n"+
			"*Total Amount:* %.3f %s\n"+
			"*Shipping Address:* %s, %s\n"+
			"*Customer Phone:* %s\n\n"+
			"Please log in to your admin panel to manage this order.",
		order.ID, order.FirstName, order.LastName, order.Email, order.Total.ToFloat(), cfg.Site.Currency, order.Address, order.City, order.Phone,
	)

	return t.sendMessage(token, chatID, text)
}

func (t *TelegramNotifier) SendAbandoned(order models.Order) error {
	cfg := config.Get()
	token := cfg.Notification.TelegramBotToken
	chatID := cfg.Notification.TelegramChatID

	if token == "" || chatID == "" {
		if order.Phone == "00000000" {
			slog.Warn("Telegram abandoned test trigger detected but configuration is missing")
			var aff models.Affiliate
			if err := db.Get().First(&aff).Error; err == nil {
				ReportWarningAffiliate(&aff, "Telegram abandoned test trigger detected but configuration is missing")
			}
		}
		return nil
	}

	if order.Phone == "00000000" {
		slog.Info("Telegram abandoned test trigger activated", "chatID", chatID)
		return t.SendTest(token, chatID)
	}

	text := fmt.Sprintf(
		"⚠️ *Abandoned Cart Alert #%d*\n\n"+
			"*Potential Customer:* %s %s\n"+
			"*Phone:* %s\n"+
			"*City:* %s\n"+
			"*Estimated Total:* %.3f %s\n\n"+
			"The customer entered their phone but hasn't finished the checkout. You might want to follow up!",
		order.ID, order.FirstName, order.LastName, order.Phone, order.City, order.Total.ToFloat(), cfg.Site.Currency,
	)

	return t.sendMessage(token, chatID, text)
}

// SendTest sends a simple verification message to the specified bot and chat.
func (t *TelegramNotifier) SendTest(token, chatID string) error {
	text := "🚀 This is a test notification from your shop. Your Telegram configuration is correct!"
	return t.sendMessage(token, chatID, text)
}

func (t *TelegramNotifier) sendMessage(token, chatID, text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]string{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status: %d", resp.StatusCode)
	}

	return nil
}
