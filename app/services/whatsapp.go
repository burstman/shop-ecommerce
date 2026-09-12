package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const whatsappCloudAPIBase = "https://graph.facebook.com/v25.0"

// ErrRecipientNotOnWhatsApp is recorded when a template send fails because the
// recipient's number is not a WhatsApp account (delivery error #131026). The
// recipient may still be billed for that message, but callers should treat the
// number as permanently unreachable and never retry it.
var ErrRecipientNotOnWhatsApp = errors.New("recipient has no whatsapp account")

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
	Type       string               `json:"type"`
	SubType    string               `json:"sub_type,omitempty"`
	Index      string               `json:"index,omitempty"`
	Parameters []cloudTemplateParam `json:"parameters"`
}

type cloudTemplateParam struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type cloudResponse struct {
	MessagingProduct string `json:"messaging_product"`
	Contacts         []struct {
		Input string `json:"input"`
		WaID  string `json:"wa_id"`
	} `json:"contacts"`
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error *struct {
		Message      string `json:"message"`
		Type         string `json:"type"`
		Code         int    `json:"code"`
		ErrorSubcode int    `json:"error_subcode"`
	} `json:"error,omitempty"`
}

type cloudTextMessage struct {
	MessagingProduct string `json:"messaging_product"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Text             struct {
		PreviewURL bool   `json:"preview_url"`
		Body       string `json:"body"`
	} `json:"text"`
}

// SendText sends a free-form text message via Meta Cloud API.
// Only works within 24 hours of the customer's last message.
func (c *WhatsAppCloudClient) SendText(phone, text string) error {
	if c.AccessToken == "" || c.PhoneNumberID == "" {
		return fmt.Errorf("whatsapp cloud api credentials not configured")
	}

	msg := cloudTextMessage{
		MessagingProduct: "whatsapp",
		To:               phone,
		Type:             "text",
	}
	msg.Text.Body = text

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

// SendTemplate sends a WhatsApp template message via Meta Cloud API.
// phone should be in international format without "+" (e.g. "21620123456").
// The Cloud API has no pre-send way to check whether the recipient has a
// WhatsApp account; Meta accepts the send and reports undeliverable numbers
// afterwards (either synchronously here or via the statuses webhook) with
// error #131026. When we see that error we return ErrRecipientNotOnWhatsApp
// so callers can suppress the number instead of retrying.
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

	return c.doSend(phone, msg)
}

// SendOrderConfirmationTemplate sends the order_confirmed template (Arabic)
// with 3 body variables and a dynamic URL button pointing to the public order
// page. orderURLSuffix is the part appended to the button's static URL base.
func (c *WhatsAppCloudClient) SendOrderConfirmationTemplate(phone, orderURLSuffix string, bodyParams []string) error {
	if c.AccessToken == "" || c.PhoneNumberID == "" {
		return fmt.Errorf("whatsapp cloud api credentials not configured")
	}

	msg := cloudTemplateMessage{
		MessagingProduct: "whatsapp",
		To:               phone,
		Type:             "template",
	}
	msg.Template.Name = "order_confirmed"
	msg.Template.Language.Code = "ar"

	comp := cloudTemplateComponent{Type: "body"}
	for _, p := range bodyParams {
		comp.Parameters = append(comp.Parameters, cloudTemplateParam{Type: "text", Text: p})
	}
	msg.Template.Components = append(msg.Template.Components, comp)

	if orderURLSuffix != "" {
		msg.Template.Components = append(msg.Template.Components, cloudTemplateComponent{
			Type:    "button",
			SubType: "url",
			Index:   "0",
			Parameters: []cloudTemplateParam{
				{Type: "text", Text: orderURLSuffix},
			},
		})
	}

	return c.doSend(phone, msg)
}

// doSend posts a message payload to the Cloud API and decodes the response.
func (c *WhatsAppCloudClient) doSend(phone string, msg cloudTemplateMessage) error {
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
		if isUndeliverableError(body) {
			return ErrRecipientNotOnWhatsApp
		}
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

// isUndeliverableError reports whether a WhatsApp API error response indicates
// a recipient that is permanently unreachable on WhatsApp (no account, blocked
// the business, or hasn't accepted the current terms). These sends are billed
// but can never succeed; they must not be retried.
func isUndeliverableError(body []byte) bool {
	var errResp struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err != nil {
		return false
	}
	return errResp.Error.Code == 131026 || errResp.Error.Code == 131030
}
