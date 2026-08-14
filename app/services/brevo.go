package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
)

func SendEmailViaBrevo(to, subject, textContent string) error {
	apiKey := os.Getenv("BREVO_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("BREVO_API_KEY not set")
	}

	fromEmail := os.Getenv("SMTP_FROM")
	if fromEmail == "" {
		fromEmail = "noreply@shop.com"
	}

	body := map[string]any{
		"sender": map[string]string{"email": fromEmail},
		"to":     []map[string]string{{"email": to}},
		"subject": subject,
		"textContent": textContent,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.brevo.com/v3/smtp/email", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("brevo API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		slog.Error("brevo API error", "status", resp.StatusCode, "body", string(respBody))
		return fmt.Errorf("brevo API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
