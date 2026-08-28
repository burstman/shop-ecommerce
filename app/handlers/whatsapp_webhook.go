package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/views/admin"
	"shopTemplate/app/views/components"

	"github.com/a-h/templ"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
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
// It saves the message to the chat system and pushes it to connected admins via WebSocket.
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

			for _, m := range change.Value.Messages {
				if m.Type != "text" {
					slog.Info("whatsapp: ignoring non-text message", "type", m.Type, "from", m.From)
					continue
				}

				phone := m.From
				customerName := ""
				for _, c := range change.Value.Contacts {
					if c.WaID == m.From {
						customerName = c.Profile.Name
						break
					}
				}

				slog.Info("whatsapp: inbound message",
					"from", phone,
					"body", m.Text.Body,
					"name", customerName,
				)

				saveAndPushWhatsAppMessage(phone, customerName, m.Text.Body)
			}
		}
	}

	// Always acknowledge to stop Meta from retrying.
	w.WriteHeader(http.StatusOK)
}

const whatsappIdentifierPrefix = "whatsapp:"

func whatsappIdentifier(phone string) string {
	return whatsappIdentifierPrefix + phone
}

// saveAndPushWhatsAppMessage stores the inbound message in the chat system
// and pushes it to all connected admin WebSocket clients in real-time.
func saveAndPushWhatsAppMessage(phone, customerName, text string) {
	id := whatsappIdentifier(phone)

	// 1. Find or create chat session
	var session models.ChatSession
	if err := db.Get().Where("identifier = ?", id).First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			session = models.ChatSession{
				Identifier:   id,
				CustomerName: customerName,
				Channel:      "whatsapp",
				Phone:        phone,
				IsActive:     true,
			}
			if err := db.Get().Create(&session).Error; err != nil {
				slog.Error("whatsapp: failed to create chat session", "phone", phone, "err", err)
				return
			}
		} else {
			slog.Error("whatsapp: failed to find chat session", "phone", phone, "err", err)
			return
		}
	} else {
		if customerName != "" && session.CustomerName == "" {
			db.Get().Model(&session).Update("customer_name", customerName)
			session.CustomerName = customerName
		}
	}

	// 2. Save the message
	chatMsg := models.ChatMessage{
		ChatSessionID: session.ID,
		Sender:        "client",
		Content:       text,
		CreatedAt:     time.Now(),
	}
	if err := db.Get().Create(&chatMsg).Error; err != nil {
		slog.Error("whatsapp: failed to save chat message", "sessionID", session.ID, "err", err)
		return
	}

	// 3. Update session timestamp
	now := time.Now()
	db.Get().Model(&session).Update("updated_at", now)
	session.UpdatedAt = now

	slog.Info("whatsapp: message saved to chat", "sessionID", session.ID, "phone", phone)

	// 4. Push to connected admin WebSocket clients
	pushWhatsAppMessageToAdmins(session, chatMsg)
}

// pushWhatsAppMessageToAdmins sends the WhatsApp message to all connected admin
// clients via WebSocket using OOB swap.
func pushWhatsAppMessageToAdmins(session models.ChatSession, msg models.ChatMessage) {
	cfg := config.Get()

	clientsMu.Lock()
	type connInfo struct {
		id   string
		conn *websocket.Conn
	}
	admins := make([]connInfo, 0, len(activeAdmins))
	for aid, conn := range activeAdmins {
		admins = append(admins, connInfo{aid, conn})
	}
	clientsMu.Unlock()

	if len(admins) == 0 {
		slog.Debug("whatsapp: no admins connected, skipping push", "sessionID", session.ID)
		return
	}

	ctx := context.Background()
	if aff := config.Get().Site.AffiliateID; aff != "" {
		// use a background context — affiliate not needed for rendering
	}

	bubbleHTML, err := componentToString(ctx, components.ChatMessageBubble(cfg, msg))
	if err != nil {
		slog.Error("whatsapp: failed to render message bubble", "err", err)
		return
	}
	sessionItemHTML, _ := componentToString(ctx, admin.ChatSessionItem(session, false, nil))

	sessionDotHTML, err := componentToString(ctx, components.ChatNotificationDot(cfg, true, msg.Content, session.ID, templ.Attributes{"hx-swap-oob": "outerHTML", "id": fmt.Sprintf("chat-notification-dot-%d", session.ID)}))
	if err != nil {
		slog.Error("whatsapp: failed to render session dot", "err", err)
		return
	}

	adminSidebarDotHTML, err := componentToString(ctx, components.ChatNotificationDot(cfg, true, msg.Content, 0, templ.Attributes{"hx-swap-oob": "outerHTML", "id": "admin-sidebar-chat-dot"}))
	if err != nil {
		return
	}

	adminTopnavDotHTML, err := componentToString(ctx, components.ChatNotificationDot(cfg, true, msg.Content, 0, templ.Attributes{"hx-swap-oob": "outerHTML", "id": "admin-topnav-chat-dot"}))
	if err != nil {
		return
	}

	for _, a := range admins {
		payload := fmt.Sprintf(
			"<div id=\"chat-messages-%d\" hx-swap-oob=\"beforeend\">%s</div>"+
				"<div id=\"delete-helper-%d\" hx-swap-oob=\"delete:#chat-session-item-%d\"></div>"+
				"<div hx-swap-oob=\"afterbegin:#sidebar-session-list\">%s</div>"+
				"%s%s%s",
			session.ID, bubbleHTML,
			session.ID, session.ID,
			sessionItemHTML,
			sessionDotHTML, adminSidebarDotHTML, adminTopnavDotHTML,
		)

		if err := a.conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
			slog.Error("whatsapp: failed to push to admin", "adminID", a.id, "err", err)
			clientsMu.Lock()
			if activeAdmins[a.id] == a.conn {
				delete(activeAdmins, a.id)
			}
			clientsMu.Unlock()
			a.conn.Close()
		}
	}
}
