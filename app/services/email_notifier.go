package services

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"os"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
)

var brevoAvailable = len(os.Getenv("BREVO_API_KEY")) > 0

// EmailNotifier implements the OrderNotifier interface using SMTP or Brevo API.
type EmailNotifier struct {
	host     string
	port     string
	username string
	password string
	from     string
}

// NewEmailNotifier creates a new instance using environment variables.
func NewEmailNotifier() *EmailNotifier {
	if os.Getenv("SMTP_PASS") == "" && os.Getenv("BREVO_API_KEY") == "" {
		slog.Warn("neither SMTP_PASS nor BREVO_API_KEY is set in environment variables")
		var aff models.Affiliate
		if err := db.Get().First(&aff).Error; err == nil {
			ReportWarningAffiliate(&aff, "SMTP_PASS is not set in environment variables")
		}
	}

	return &EmailNotifier{
		host:     os.Getenv("SMTP_HOST"),
		port:     os.Getenv("SMTP_PORT"),
		username: os.Getenv("SMTP_USER"),
		password: os.Getenv("SMTP_PASS"),
		from:     os.Getenv("SMTP_FROM"),
	}
}

func (e *EmailNotifier) Name() string {
	if brevoAvailable {
		return "Brevo API"
	}
	return "Brevo SMTP"
}

func (e *EmailNotifier) send(recipient, subject, body string) error {
	if brevoAvailable {
		return SendEmailViaBrevo(recipient, subject, body)
	}
	auth := smtp.PlainAuth("", e.username, e.password, e.host)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-version: 1.0\r\nContent-Type: text/plain; charset=\"UTF-8\"\r\n\r\n%s", e.from, recipient, subject, body)
	return smtp.SendMail(e.host+":"+e.port, auth, e.from, []string{recipient}, []byte(msg))
}

// Send formats a basic MIME email and sends it via the configured SMTP server.
func (e *EmailNotifier) Send(order models.Order) error {
	if order.Phone == "00000000" {
		cfg := config.Get()
		adminEmail := cfg.Notification.AdminEmailRecipient
		if adminEmail == "" {
			adminEmail = e.from
		}
		slog.Info("Email test trigger activated", "recipient", adminEmail)
		return e.SendTest(adminEmail)
	}

	cfg := config.Get()

	adminEmail := cfg.Notification.AdminEmailRecipient
	if adminEmail == "" {
		adminEmail = e.from
	}

	customerSubject := fmt.Sprintf("Order Confirmation #%d - %s", order.ID, cfg.Site.Name)
	customerBody := fmt.Sprintf(
		"Hello %s %s,\n\n"+
			"Thank you for your order! We have received it and are currently processing it.\n\n"+
			"Order Summary:\n"+
			"Order Number: #%d\n"+
			"Total: %.3f %s\n"+
			"Shipping to: %s, %s\n\n"+
			"We will notify you once your order has been shipped.",
		order.FirstName, order.LastName, order.ID, order.Total.ToFloat(), cfg.Site.Currency, order.Address, order.City,
	)

	if err := e.send(order.Email, customerSubject, customerBody); err != nil {
		slog.Error("failed to send customer confirmation email", "err", err, "orderID", order.ID)
	}

	adminSubject := fmt.Sprintf("New Order #%d from %s %s", order.ID, order.FirstName, order.LastName)
	adminBody := fmt.Sprintf(
		"Hello,\n\n"+
			"You have a new order from %s %s (%s).\n\n"+
			"Order Details:\n"+
			"Order ID: #%d\n"+
			"Total Amount: %.3f %s\n"+
			"Shipping Address: %s, %s\n"+
			"Customer Phone: %s\n\n"+
			"Please log in to your admin panel to manage this order.",
		order.FirstName, order.LastName, order.Email, order.ID, order.Total.ToFloat(), cfg.Site.Currency, order.Address, order.City, order.Phone,
	)

	return e.send(adminEmail, adminSubject, adminBody)
}

func (e *EmailNotifier) SendAbandoned(order models.Order) error {
	if order.Phone == "00000000" {
		cfg := config.Get()
		adminEmail := cfg.Notification.AdminEmailRecipient
		if adminEmail == "" {
			adminEmail = e.from
		}
		slog.Info("Email (abandoned) test trigger activated", "recipient", adminEmail)
		return e.SendTest(adminEmail)
	}

	cfg := config.Get()

	adminEmail := cfg.Notification.AdminEmailRecipient
	if adminEmail == "" {
		adminEmail = e.from
	}

	subject := fmt.Sprintf("[Abandoned Cart] Potential Order #%d", order.ID)
	body := fmt.Sprintf(
		"Hello,\n\n"+
			"A customer started a checkout but hasn't finished yet.\n\n"+
			"Partial Details:\n"+
			"Order ID: #%d\n"+
			"Potential Customer: %s %s\n"+
			"Phone: %s\n"+
			"City: %s\n"+
			"Estimated Total: %.3f %s\n\n"+
			"You might want to check the admin panel for more details.",
		order.ID, order.FirstName, order.LastName, order.Phone, order.City, order.Total.ToFloat(), cfg.Site.Currency,
	)

	return e.send(adminEmail, subject, body)
}

// SendTest sends a simple verification email to the specified recipient.
func (e *EmailNotifier) SendTest(recipient string) error {
	subject := "[Test] Shop Notification System"
	body := "This is a test email to verify your shop's notification settings. If you received this, your SMTP configuration is working correctly."
	return e.send(recipient, subject, body)
}
