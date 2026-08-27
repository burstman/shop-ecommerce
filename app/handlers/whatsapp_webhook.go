package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"shopTemplate/app/config"
	"shopTemplate/app/services"
)

// HandleWhatsAppWebhook handles GET (verification handshake) and POST (events)
// requests from the Meta WhatsApp Cloud API.
func HandleWhatsAppWebhook(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()

	if r.Method == http.MethodGet {
		handleWhatsAppVerification(w, r, cfg)
		return
	}

	if r.Method == http.MethodPost {
		handleWhatsAppIncoming(w, r, cfg)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// handleWhatsAppVerification responds to Meta's subscription verification handshake.
// Meta sends: ?hub.mode=subscribe&hub.verify_token=<TOKEN>&hub.challenge=<CHALLENGE>
func handleWhatsAppVerification(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token != "" && token == cfg.WhatsApp.VerifyToken {
		slog.Info("whatsapp: webhook verified")
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(challenge))
		return
	}

	slog.Warn("whatsapp: webhook verification failed",
		"mode", mode,
		"verify_token", token,
		"expected", cfg.WhatsApp.VerifyToken,
	)
	http.Error(w, "forbidden", http.StatusForbidden)
}

// whatsAppMessage is the top-level structure of an incoming webhook event.
type whatsAppMessage struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaID string `json:"wa_id"`
				} `json:"contacts"`
				Messages []struct {
					From string `json:"from"`
					ID   string `json:"id"`
					Type string `json:"type"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
			Field string `json:"field"`
		} `json:"changes"`
	} `json:"entry"`
}

// handleWhatsAppIncoming processes incoming messages from customers on WhatsApp.
func handleWhatsAppIncoming(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var msg whatsAppMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		slog.Error("whatsapp: failed to parse webhook payload", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, entry := range msg.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			phoneNumberID := change.Value.Metadata.PhoneNumberID
			for _, m := range change.Value.Messages {
				slog.Info("whatsapp: inbound message",
					"from", m.From,
					"type", m.Type,
					"phone_number_id", phoneNumberID,
				)

				services.HandleWhatsAppInboundMessage(phoneNumberID, m.From, m.Type, m.Text.Body)
			}
		}
	}

	// Always acknowledge to stop Meta from retrying.
	w.WriteHeader(http.StatusOK)
}