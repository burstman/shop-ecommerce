package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const d360BaseURL = "https://waba-v2.360dialog.io"

type D360Client struct {
	APIKey     string
	httpClient *http.Client
}

func NewD360Client(apiKey string) *D360Client {
	return &D360Client{
		APIKey: apiKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type templateMessage struct {
	MessagingProduct string `json:"messaging_product"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Template         struct {
		Name     string `json:"name"`
		Language struct {
			Code string `json:"code"`
		} `json:"language"`
		Components []templateComponent `json:"components,omitempty"`
	} `json:"template"`
}

type templateComponent struct {
	Type       string             `json:"type"`
	SubType    string             `json:"sub_type,omitempty"`
	Parameters []templateParameter `json:"parameters"`
}

type templateParameter struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type d360Response struct {
	MessagingProduct string `json:"messaging_product"`
	Contacts         []struct {
		Input   string `json:"input"`
		WaID    string `json:"wa_id"`
	} `json:"contacts"`
	Messages []struct {
		ID            string `json:"id"`
		MessageStatus string `json:"message_status"`
	} `json:"messages"`
	Error *struct {
		Message string `json:"error"`
	} `json:"error,omitempty"`
}

// SendTemplate sends a WhatsApp template message to the given phone number.
// phone should be in international format without "+" (e.g. "21620123456").
func (c *D360Client) SendTemplate(phone, templateName, langCode string, params []string) error {
	if c.APIKey == "" {
		return fmt.Errorf("360dialog api key is not configured")
	}

	msg := templateMessage{
		MessagingProduct: "whatsapp",
		To:               phone,
		Type:             "template",
	}
	msg.Template.Name = templateName
	msg.Template.Language.Code = langCode

	if len(params) > 0 {
		comp := templateComponent{
			Type: "body",
		}
		for _, p := range params {
			comp.Parameters = append(comp.Parameters, templateParameter{
				Type: "text",
				Text: p,
			})
		}
		msg.Template.Components = append(msg.Template.Components, comp)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, d360BaseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("D360-API-KEY", c.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("360dialog: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("360dialog: http %d: %s", resp.StatusCode, string(body))
	}

	var d360Resp d360Response
	if err := json.Unmarshal(body, &d360Resp); err != nil {
		return fmt.Errorf("360dialog: failed to decode response: %w", err)
	}
	if d360Resp.Error != nil {
		return fmt.Errorf("360dialog: %s", d360Resp.Error.Message)
	}
	return nil
}
