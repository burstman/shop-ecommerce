package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"shopTemplate/app/config"
	"shopTemplate/app/models"
	"strings"
	"time"
)

type FacebookCAPIService struct {
	cfg *config.Config
}

func NewFacebookCAPIService() *FacebookCAPIService {
	return &FacebookCAPIService{
		cfg: config.Get(),
	}
}

type capiEvent struct {
	EventName      string         `json:"event_name"`
	EventTime      int64          `json:"event_time"`
	ActionSource   string         `json:"action_source"`
	UserData       capiUserData   `json:"user_data"`
	CustomData     capiCustomData `json:"custom_data"`
	EventSourceURL string         `json:"event_source_url,omitempty"`
}

type capiUserData struct {
	Email     []string `json:"em,omitempty"`
	Phone     []string `json:"ph,omitempty"`
	FirstName []string `json:"fn,omitempty"`
	LastName  []string `json:"ln,omitempty"`
	ClientIP  string   `json:"client_ip_address,omitempty"`
	ClientUA  string   `json:"client_user_agent,omitempty"`
}

type capiCustomData struct {
	Currency    string   `json:"currency,omitempty"`
	Value       *float64 `json:"value,omitempty"`
	ContentType string   `json:"content_type,omitempty"`
	ContentName string   `json:"content_name,omitempty"`
	ContentIDs  []string `json:"content_ids,omitempty"`
}

func (s *FacebookCAPIService) SendPageViewEvent(url, ip, ua string) {
	s.sendEvents([]capiEvent{{
		EventName:    "PageView",
		EventTime:    time.Now().Unix(),
		ActionSource: "website",
		EventSourceURL: url,
		UserData: capiUserData{
			ClientIP: ip,
			ClientUA: ua,
		},
	}})
}

func (s *FacebookCAPIService) SendAddToCartEvent(productID uint, productName string, price float64, url, ip, ua string) {
	s.sendEvents([]capiEvent{{
		EventName:    "AddToCart",
		EventTime:    time.Now().Unix(),
		ActionSource: "website",
		EventSourceURL: url,
		UserData: capiUserData{
			ClientIP: ip,
			ClientUA: ua,
		},
		CustomData: capiCustomData{
			Currency:    s.getCurrency(),
			Value:       &price,
			ContentType: "product",
			ContentName: productName,
			ContentIDs:  []string{fmt.Sprint(productID)},
		},
	}})
}

func (s *FacebookCAPIService) SendPurchaseEvent(order models.Order) {
	userData := capiUserData{
		Email:     []string{hashString(order.Email)},
		Phone:     []string{hashString(order.Phone)},
		FirstName: []string{hashString(order.FirstName)},
		LastName:  []string{hashString(order.LastName)},
	}

	var value *float64
	if s.cfg.FacebookPixel.TrackPurchaseValue {
		v := order.Total.ToFloat()
		value = &v
	}

	event := capiEvent{
		EventName:    "Purchase",
		EventTime:    time.Now().Unix(),
		ActionSource: "website",
		UserData:     userData,
		CustomData: capiCustomData{
			Currency: s.getCurrency(),
			Value:    value,
		},
	}

	s.sendEvents([]capiEvent{event})
}

func (s *FacebookCAPIService) SendInitiateCheckoutEvent(order models.Order) {
	userData := capiUserData{
		Email:     []string{hashString(order.Email)},
		Phone:     []string{hashString(order.Phone)},
		FirstName: []string{hashString(order.FirstName)},
		LastName:  []string{hashString(order.LastName)},
	}

	v := order.Total.ToFloat()
	event := capiEvent{
		EventName:    "InitiateCheckout",
		EventTime:    time.Now().Unix(),
		ActionSource: "website",
		UserData:     userData,
		CustomData: capiCustomData{
			Currency: s.getCurrency(),
			Value:    &v,
		},
	}

	s.sendEvents([]capiEvent{event})
}

func (s *FacebookCAPIService) sendEvents(events []capiEvent) {
	if s.cfg.FacebookPixel.PixelID == "" || s.cfg.FacebookPixel.AccessToken == "" {
		return
	}

	payload := map[string]interface{}{
		"data": events,
	}
	if s.cfg.FacebookPixel.TestEventCode != "" {
		payload["test_event_code"] = s.cfg.FacebookPixel.TestEventCode
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal Facebook CAPI payload", "err", err)
		return
	}

	url := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/events?access_token=%s",
		s.cfg.FacebookPixel.PixelID, s.cfg.FacebookPixel.AccessToken)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		slog.Error("failed to send Facebook CAPI events", "err", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&result)
		slog.Error("Facebook CAPI error response", "status", resp.StatusCode, "result", result)
	} else {
		slog.Info("Facebook CAPI events sent successfully", "count", len(events), "first_event", events[0].EventName)
	}
}

func (s *FacebookCAPIService) getCurrency() string {
	if s.cfg.Site.Currency != "" {
		return s.cfg.Site.Currency
	}
	return "TND"
}

func hashString(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ToLower(strings.TrimSpace(s))
	hash := sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}
