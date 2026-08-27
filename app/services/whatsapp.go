package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const whatsappCloudAPIBase = "https://graph.facebook.com/v25.0"

type WhatsAppCloudClient struct {
	PhoneNumberID string
	AccessToken   string
	httpClient    *http.Client
}

func NewWhatsAppCloudClient(phoneNumberID, accessToken string) *WhatsAppCloudClient {
	return &WhatsAppCloudClient{
		PhoneNumberID: phoneNumberID,
		AccessToken:   accessToken,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type cloudTemplateMessage struct {
	MessagingProduct string `json:"messaging_product"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Template         struct {
		Name     string `json:"name"`
		Language struct {
			Code string `json:"code"`
		} `json:"language"`
		Components []cloudTemplateComponent `json:"components,omitempty"`
	} `json:"template"`
}

type cloudTemplateComponent struct {
	Type       string              `json:"type"`
	Parameters []cloudTemplateParam `json:"parameters"`
}

type cloudTemplateParam struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type cloudResponse struct {
	MessagingProduct string `json:"messaging_product"`
	Contacts         []struct {
		Input   string `json:"input"`
		WaID    string `json:"wa_id"`
	} `json:"contacts"`
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error *struct {
		Message    string `json:"message"`
		Type       string `json:"type"`
		Code       int    `json:"code"`
		ErrorSubcode int  `json:"error_subcode"`
	} `json:"error,omitempty"`
}

// SendTemplate sends a WhatsApp template message via Meta Cloud API.
// phone should be in international format without "+" (e.g. "21620123456").
func (c *WhatsAppCloudClient) SendTemplate(phone, templateName, langCode string, params []string) error {
	if c.AccessToken == "" || c.PhoneNumberID == "" {
		return fmt.Errorf("whatsapp cloud api credentials not configured")
	}

	msg := cloudTemplateMessage{
		MessagingProduct: "whatsapp",
		To:               phone,
		Type:             "template",
	}
	msg.Template.Name = templateName
	msg.Template.Language.Code = langCode

	if len(params) > 0 {
		comp := cloudTemplateComponent{
			Type: "body",
		}
		for _, p := range params {
			comp.Parameters = append(comp.Parameters, cloudTemplateParam{
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

	url := fmt.Sprintf("%s/%s/messages", whatsappCloudAPIBase, c.PhoneNumberID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp cloud: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("whatsapp cloud: http %d: %s", resp.StatusCode, string(body))
	}

	var apiResp cloudResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("whatsapp cloud: failed to decode response: %w", err)
	}
	if apiResp.Error != nil {
		return fmt.Errorf("whatsapp cloud: %s (code %d)", apiResp.Error.Message, apiResp.Error.Code)
	}
	return nil
}
