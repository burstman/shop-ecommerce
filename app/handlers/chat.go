package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/views/admin"
	"shopTemplate/app/views/components"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			// Allow if ALLOWED_ORIGIN is set to "*" or empty
			allowedOrigin := strings.TrimSpace(os.Getenv("ALLOWED_ORIGIN"))
			if allowedOrigin == "" || allowedOrigin == "*" {
				return true
			}
			// Exact match against ALLOWED_ORIGIN
			if origin == allowedOrigin {
				return true
			}
			// Allow if origin matches the request host (same-origin)
			parsedOrigin, err := url.Parse(origin)
			if err == nil && parsedOrigin.Host == r.Host {
				return true
			}
			return false
		},
	}
	// Active connections mapped by chat identifier
	activeClients = make(map[string]*websocket.Conn)
	activeAdmins  = make(map[string]*websocket.Conn) // For admin dashboard
	clientsMu     sync.Mutex

	bufferPool = sync.Pool{
		New: func() any {
			return new(bytes.Buffer)
		},
	}
)

func componentToString(ctx context.Context, comp templ.Component) (string, error) {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)
	if err := comp.Render(ctx, buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// getChatIdentifier retrieves the unique ID from a cookie or creates a new one for guests.
func getChatIdentifier(kit *kit.Kit) string {
	cookie, err := kit.Request.Cookie("chat_id")
	if err == nil {
		return cookie.Value
	}

	id := uuid.New().String()
	http.SetCookie(kit.Response, &http.Cookie{
		Name:     "chat_id",
		Value:    id,
		Path:     "/",
		Expires:  time.Now().Add(365 * 24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return id
}

// HandleChatWS upgrades the connection to WebSocket and keeps it open.
func HandleChatWS(kit *kit.Kit) error {
	id := getChatIdentifier(kit)
	user, _ := kit.Auth().(models.AuthUser)
	isAdmin := user.Role == "admin"

	conn, err := upgrader.Upgrade(kit.Response, kit.Request, nil)
	if err != nil {
		return err
	}

	if !isAdmin {
		var session models.ChatSession
		if err := db.Get().Where("identifier = ?", id).Limit(1).Find(&session).Error; err == nil && session.IsBanned {
			// Send a visual notice via WebSocket OOB swap before closing
			notice, err := componentToString(kit.Request.Context(), components.ChatBanOOB())
			if err == nil {
				conn.WriteMessage(websocket.TextMessage, []byte(notice))
			}
			conn.Close()
			return nil
		}
	}

	slog.Info("websocket connected", "id", id, "isAdmin", isAdmin, "user", user.Email)

	clientsMu.Lock()
	if isAdmin {
		activeAdmins["admin-"+user.Email] = conn
	} else {
		activeClients[id] = conn
	}
	clientsMu.Unlock()

	if !isAdmin {
		var session models.ChatSession
		if err := db.Get().Where("identifier = ?", id).FirstOrCreate(&session, models.ChatSession{Identifier: id}).Error; err != nil {
			slog.Error("failed to find or create chat session", "err", err, "id", id)
		}
		fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)

		if user.LoggedIn && fullName != "" && session.CustomerName != fullName {
			updateSessionName(kit.Request.Context(), id, fullName)
		}

		broadcastStatusUpdate(kit.Request.Context(), id, true)
	}

	// Start a heartbeat ticker to prevent idle timeouts from proxies
	stopPing := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-stopPing:
				return
			}
		}
	}()

	defer func() {
		slog.Info("websocket disconnected", "id", id, "isAdmin", isAdmin, "user", user.Email)
		clientsMu.Lock()
		if isAdmin {
			if activeAdmins["admin-"+user.Email] == conn {
				delete(activeAdmins, "admin-"+user.Email)
			}
		} else {
			if activeClients[id] == conn {
				delete(activeClients, id)
			}
		}
		clientsMu.Unlock()
		conn.Close()

		if !isAdmin {
			// Use background context as request context may be canceled during closure
			broadcastStatusUpdate(context.Background(), id, false)
		}
	}()

	// Keep connection alive and handle incoming messages if not using ws-send
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			close(stopPing)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Error("websocket read error", "err", err, "id", id)
			}
			break
		}

		if !isAdmin {
			var data struct {
				Type     string `json:"type"`
				IsTyping bool   `json:"is_typing"`
				Name     string `json:"name"`
			}
			if err := json.Unmarshal(msg, &data); err == nil {
				if data.Type == "typing" {
					broadcastTypingUpdate(kit.Request.Context(), id, data.IsTyping)
				} else if data.Type == "set_name" && data.Name != "" {
					slog.Info("received set_name request", "id", id, "name", data.Name)
					updateSessionName(kit.Request.Context(), id, data.Name)
				}
			}
		}
	}
	return nil
}

// HandleChatFetchMessages returns the full list of messages for the client.
func HandleChatFetchMessages(kit *kit.Kit) error {
	id := getChatIdentifier(kit)
	user, _ := kit.Auth().(models.AuthUser)

	var session models.ChatSession
	// We preload messages and order them by creation date
	err := db.Get().
		Preload("Messages", "1=1 ORDER BY created_at ASC").
		Where("identifier = ?", id).
		Limit(1).
		Find(&session).Error

	if err != nil {
		return err
	}

	// Auto-sync name if logged in
	fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if user.LoggedIn && fullName != "" && session.CustomerName != fullName {
		updateSessionName(kit.Request.Context(), id, fullName)
	}

	return kit.Render(components.ChatMessages(config.FromContext(kit.Request.Context()), session.Messages))
}

// HandleChatSend saves a message from the client.
func HandleChatSend(kit *kit.Kit) error {
	id := getChatIdentifier(kit)
	user, _ := kit.Auth().(models.AuthUser)
	content := kit.Request.FormValue("message")
	if strings.TrimSpace(content) == "" {
		return nil
	}

	var session models.ChatSession
	if err := db.Get().Where("identifier = ?", id).FirstOrCreate(&session, models.ChatSession{Identifier: id}).Error; err != nil {
		return err
	}

	fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if user.LoggedIn && fullName != "" && session.CustomerName != fullName {
		updateSessionName(kit.Request.Context(), id, fullName)
	}

	if session.IsBanned {
		return kit.Render(components.ChatBanNotice())
	}

	msg := models.ChatMessage{
		ChatSessionID: session.ID,
		Sender:        "client",
		Content:       content,
		CreatedAt:     time.Now(),
	}

	if err := db.Get().Create(&msg).Error; err != nil {
		return err
	}

	logID := id
	if user.LoggedIn {
		fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)
		logID = fmt.Sprintf("%s (%s)", fullName, user.Email)
	}
	slog.Info("chat message received", "from", logID, "content", content)

	// Update session activity for sidebar sorting and ensure it's saved
	now := time.Now()
	if err := db.Get().Model(&session).Update("updated_at", now).Error; err != nil {
		return err
	}
	session.UpdatedAt = now

	// Push to all connected admins
	cfg := config.FromContext(kit.Request.Context())

	// Generate HTML once outside the loop for efficiency
	bubbleHTML, err := componentToString(kit.Request.Context(), components.ChatMessageBubble(cfg, msg)) // Message bubble
	if err != nil {
		return err
	}
	sessionItemHTML, _ := componentToString(kit.Request.Context(), admin.ChatSessionItem(session, true, nil)) // Sidebar item (no OOB on itself)

	// Generate HTML for the session-specific dot (e.g., next to the session in the sidebar list)
	sessionDotHTML, err := componentToString(kit.Request.Context(), components.ChatNotificationDot(cfg, true, msg.Content, session.ID, templ.Attributes{"hx-swap-oob": "outerHTML", "id": fmt.Sprintf("chat-notification-dot-%d", session.ID)}))
	if err != nil {
		return err
	}

	// Generate HTML for the global notification dot in the admin sidebar
	adminSidebarDotHTML, err := componentToString(kit.Request.Context(), components.ChatNotificationDot(cfg, true, msg.Content, 0, templ.Attributes{"hx-swap-oob": "outerHTML", "id": "admin-sidebar-chat-dot"}))
	if err != nil {
		return err
	}

	// Generate HTML for the global notification dot in the top navigation (if it exists)
	adminTopnavDotHTML, err := componentToString(kit.Request.Context(), components.ChatNotificationDot(cfg, true, msg.Content, 0, templ.Attributes{"hx-swap-oob": "outerHTML", "id": "admin-topnav-chat-dot"}))
	if err != nil {
		return err
	}

	// Collect active admin connections
	clientsMu.Lock()
	type connInfo struct {
		id   string
		conn *websocket.Conn
	}
	admins := make([]connInfo, 0, len(activeAdmins))
	for aid, conn := range activeAdmins {
		admins = append(admins, connInfo{aid, conn})
	}
	slog.Debug("broadcasting message to admins", "count", len(admins), "sessionID", session.ID)
	clientsMu.Unlock()

	for _, a := range admins {
		var payload strings.Builder
		// 1. Append the message bubble to the chat window
		payload.WriteString(fmt.Sprintf("<div id=\"chat-messages-%d\" hx-swap-oob=\"beforeend\">%s</div>", session.ID, bubbleHTML))

		// 2. Move this session to the top of the sidebar list
		payload.WriteString(fmt.Sprintf("<div id=\"delete-helper-%d\" hx-swap-oob=\"delete:#chat-session-item-%d\"></div>", session.ID, session.ID))
		// Wrap the session item in a div for the afterbegin swap
		payload.WriteString(fmt.Sprintf("<div hx-swap-oob=\"afterbegin:#sidebar-session-list\">%s</div>", sessionItemHTML))

		// 3. Update the red notification dots and play the sound
		payload.WriteString(sessionDotHTML)      // Update session-specific dot
		payload.WriteString(adminSidebarDotHTML) // Update global sidebar dot
		payload.WriteString(adminTopnavDotHTML)  // Update global topnav dot

		if err := a.conn.WriteMessage(websocket.TextMessage, []byte(payload.String())); err != nil {
			slog.Error("failed to push to admin", "adminID", a.id, "err", err)
			clientsMu.Lock()
			if activeAdmins[a.id] == a.conn {
				delete(activeAdmins, a.id)
			}
			clientsMu.Unlock()
			a.conn.Close()
		}
	}

	// We return only the new bubble so HTMX can swap it with 'beforeend'
	return kit.Render(components.ChatMessageBubble(config.FromContext(kit.Request.Context()), msg))
}

// HandleAdminChatIndex displays the list of all chat sessions for the admin.
func HandleAdminChatIndex(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	var sessions []models.ChatSession
	// Get sessions ordered by most recent message
	if err := db.Get().Order("updated_at desc").Find(&sessions).Error; err != nil {
		return err
	}

	clientsMu.Lock()
	onlineMap := make(map[string]bool)
	for id := range activeClients {
		onlineMap[id] = true
	}
	for _, s := range sessions {
		if !onlineMap[s.Identifier] && time.Since(s.UpdatedAt) < 15*time.Second {
			onlineMap[s.Identifier] = true
		}
	}
	clientsMu.Unlock()

	if kit.Request.Header.Get("HX-Request") == "true" {
		return kit.Render(admin.ChatIndex(sessions, onlineMap))
	}

	return RenderWithLayout(kit, admin.ChatIndex(sessions, onlineMap))
}

// HandleAdminChatSidebar returns only the sidebar session list for polling.
func HandleAdminChatSidebar(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	var sessions []models.ChatSession
	if err := db.Get().Order("updated_at desc").Find(&sessions).Error; err != nil {
		return err
	}

	clientsMu.Lock()
	onlineMap := make(map[string]bool)
	for id := range activeClients {
		onlineMap[id] = true
	}
	for _, s := range sessions {
		if !onlineMap[s.Identifier] && time.Since(s.UpdatedAt) < 15*time.Second {
			onlineMap[s.Identifier] = true
		}
	}
	clientsMu.Unlock()

	// Prevent caching of polling results
	kit.Response.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")

	return kit.Render(admin.ChatSessionList(sessions, onlineMap))
}

// HandleAdminChatMessages returns only the messages for a specific session for polling.
func HandleAdminChatMessages(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	sessionIDStr := chi.URLParam(kit.Request, "id")
	sessionID, err := strconv.Atoi(sessionIDStr)
	if err != nil {
		return kit.Render(components.ChatMessages(config.FromContext(kit.Request.Context()), []models.ChatMessage{}))
	}

	var session models.ChatSession
	err = db.Get().
		Preload("Messages", "1=1 ORDER BY created_at ASC").
		First(&session, sessionID).Error
	if err != nil {
		// Return empty list instead of 204 to keep HTMX target happy
		return kit.Render(components.ChatMessages(config.FromContext(kit.Request.Context()), []models.ChatMessage{}))
	}

	// Prevent caching of polling results
	kit.Response.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")

	return kit.Render(components.ChatMessages(config.FromContext(kit.Request.Context()), session.Messages))
}

// HandleAdminChatShow displays the messages for a specific session.
func HandleAdminChatShow(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	sessionIDStr := chi.URLParam(kit.Request, "id")
	sessionID, err := strconv.Atoi(sessionIDStr)
	if err != nil {
		return err
	}

	var session models.ChatSession
	err = db.Get().
		Preload("Messages", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		First(&session, sessionID).Error

	if err != nil {
		return err
	}

	cfg := config.FromContext(kit.Request.Context())

	if kit.Request.Header.Get("HX-Request") == "true" {
		clientsMu.Lock()
		_, wsOnline := activeClients[session.Identifier]
		isOnline := wsOnline || time.Since(session.UpdatedAt) < 15*time.Second
		clientsMu.Unlock()
		return kit.Render(admin.ChatDetail(session, cfg, isOnline))
	}

	// Fallback for full page refresh: render the main index
	return HandleAdminChatIndex(kit)
}

// HandleAdminChatSend allows an admin to respond to a specific session.
func HandleAdminChatSend(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	sessionIDStr := chi.URLParam(kit.Request, "id")
	sessionID, err := strconv.Atoi(sessionIDStr)
	if err != nil {
		return err
	}
	content := kit.Request.FormValue("message")

	if strings.TrimSpace(content) == "" {
		return nil
	}

	msg := models.ChatMessage{
		ChatSessionID: uint(sessionID),
		Sender:        "admin",
		Content:       content,
		CreatedAt:     time.Now(),
	}

	if err := db.Get().Create(&msg).Error; err != nil {
		return err
	}

	// Update session activity
	if err := db.Get().Model(&models.ChatSession{}).Where("id = ?", sessionID).Update("updated_at", time.Now()).Error; err != nil {
		return err
	}

	// Real-time push to participants
	var session models.ChatSession
	if err := db.Get().First(&session, sessionID).Error; err == nil {
		cfg := config.FromContext(kit.Request.Context())
		bubbleHTML, err := componentToString(kit.Request.Context(), components.ChatMessageBubble(cfg, msg))
		if err != nil {
			return err
		}

		clientsMu.Lock()
		type connInfo struct {
			id   string
			conn *websocket.Conn
		}

		clientConn, isOnline := activeClients[session.Identifier]
		currentAdminID := "admin-" + user.Email

		admins := make([]connInfo, 0, len(activeAdmins))
		for aid, conn := range activeAdmins {
			admins = append(admins, connInfo{aid, conn})
		}
		clientsMu.Unlock()

		// Push to client
		if isOnline {
			// Wrap the message in the expected container for HTMX OOB swap on client side
			payload := fmt.Sprintf("<div id=\"chat-messages\" hx-swap-oob=\"beforeend\">%s</div>", bubbleHTML)
			if err := clientConn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
				clientsMu.Lock()
				delete(activeClients, session.Identifier)
				clientsMu.Unlock()
			}
		}

		// Push to all admins to sync their views and sidebar
		// Remove hx-swap-oob from the component itself
		sessionItemHTML, err := componentToString(kit.Request.Context(), admin.ChatSessionItem(session, isOnline, nil))
		if err != nil {
			return err
		}
		// Generate HTML for the global notification dot in the admin sidebar
		adminSidebarDotHTML, err := componentToString(kit.Request.Context(), components.ChatNotificationDot(cfg, false, "", 0, templ.Attributes{"hx-swap-oob": "outerHTML", "id": "admin-sidebar-chat-dot"}))
		if err != nil {
			return err
		}
		// Generate HTML for the global notification dot in the top navigation (if it exists)
		adminTopnavDotHTML, err := componentToString(kit.Request.Context(), components.ChatNotificationDot(cfg, false, "", 0, templ.Attributes{"hx-swap-oob": "outerHTML", "id": "admin-topnav-chat-dot"}))
		if err != nil {
			return err
		}
		// This dotHTML is for the session-specific dot for the current session being responded to.
		dotHTML, err := componentToString(kit.Request.Context(), components.ChatNotificationDot(cfg, false, "", uint(sessionID), templ.Attributes{"hx-swap-oob": "outerHTML", "id": fmt.Sprintf("chat-notification-dot-%d", sessionID)}))
		if err != nil {
			return err
		}
		for _, a := range admins {
			// We exclude the sender to avoid duplicate messages if the admin's
			// UI is already updating via standard hx-post response
			if a.id == currentAdminID {
				continue
			}

			var payload strings.Builder
			payload.WriteString(fmt.Sprintf("<div id=\"chat-messages-%d\" hx-swap-oob=\"beforeend\">%s</div>", session.ID, bubbleHTML))
			payload.WriteString(fmt.Sprintf("<div id=\"delete-helper-%d\" hx-swap-oob=\"delete:#chat-session-item-%d\"></div>", session.ID, session.ID))
			// Wrap the session item in a div for the afterbegin swap
			payload.WriteString(fmt.Sprintf("<div hx-swap-oob=\"afterbegin:#sidebar-session-list\">%s</div>", sessionItemHTML)) // Update session list order
			payload.WriteString(adminSidebarDotHTML)                                                                            // Update global sidebar dot
			payload.WriteString(adminTopnavDotHTML)                                                                             // Update global topnav dot
			payload.WriteString(dotHTML)

			if err := a.conn.WriteMessage(websocket.TextMessage, []byte(payload.String())); err != nil {
				go func(id string, c *websocket.Conn) {
					clientsMu.Lock()
					if activeAdmins[id] == c {
						delete(activeAdmins, id)
					}
					clientsMu.Unlock()
				}(a.id, a.conn)
			}
		}
	}

	return kit.Render(components.ChatMessageBubble(config.FromContext(kit.Request.Context()), msg))
}

func broadcastStatusUpdate(ctx context.Context, identifier string, isOnline bool) {
	var session models.ChatSession
	// Use Find instead of First to avoid "record not found" errors in logs
	if err := db.Get().Where("identifier = ?", identifier).Limit(1).Find(&session).Error; err != nil || session.ID == 0 {
		return
	}

	clientsMu.Lock()
	defer clientsMu.Unlock()

	dotHTML, err := componentToString(ctx, components.ChatStatusDot(isOnline, session.ID))
	if err != nil {
		return
	}
	statusHTML, err := componentToString(ctx, components.ChatStatusIndicator(isOnline, session.ID))
	if err != nil {
		return
	}

	var payload strings.Builder
	payload.WriteString(fmt.Sprintf("<div id=\"client-dot-%d\" hx-swap-oob=\"outerHTML\">%s</div>", session.ID, dotHTML))
	payload.WriteString(fmt.Sprintf("<div id=\"client-status-%d\" hx-swap-oob=\"outerHTML\">%s</div>", session.ID, statusHTML))

	// If the user is connecting, ensure they appear at the top of the admin sidebar immediately
	if isOnline {
		itemHTML, _ := componentToString(ctx, admin.ChatSessionItem(session, isOnline, nil))
		// Delete existing item if present, then prepend to top of list
		payload.WriteString(fmt.Sprintf("<div id=\"delete-helper-%d\" hx-swap-oob=\"delete:#chat-session-item-%d\"></div>", session.ID, session.ID))
		payload.WriteString(fmt.Sprintf("<div hx-swap-oob=\"afterbegin:#sidebar-session-list\">%s</div>", itemHTML))
	}

	msg := []byte(payload.String())
	for _, adminConn := range activeAdmins {
		adminConn.WriteMessage(websocket.TextMessage, msg)
	}
}

func updateSessionName(ctx context.Context, identifier string, name string) {
	var session models.ChatSession
	// Ensure the session exists so the name is saved even if set before the first message
	if err := db.Get().Where("identifier = ?", identifier).FirstOrCreate(&session, models.ChatSession{Identifier: identifier}).Error; err != nil {
		return
	}

	slog.Info("updating session name", "sessionID", session.ID, "oldName", session.CustomerName, "newName", name)

	session.CustomerName = name
	if err := db.Get().Model(&session).Update("customer_name", name).Error; err != nil {
		slog.Error("failed to update customer_name in db", "err", err, "sessionID", session.ID)
		return
	}

	clientsMu.Lock()
	defer clientsMu.Unlock()

	isOnline := activeClients[identifier] != nil

	// Update sidebar item using Upsert pattern
	itemHTML, _ := componentToString(ctx, admin.ChatSessionItem(session, isOnline, nil))

	var payload strings.Builder
	payload.WriteString(fmt.Sprintf("<div id=\"delete-helper-%d\" hx-swap-oob=\"delete:#chat-session-item-%d\"></div>", session.ID, session.ID))
	payload.WriteString(fmt.Sprintf("<div hx-swap-oob=\"afterbegin:#sidebar-session-list\">%s</div>", itemHTML))

	// Update detail header
	payload.WriteString(fmt.Sprintf(`<h3 id="chat-header-%d" hx-swap-oob="outerHTML" class="font-bold text-gray-900">%s</h3>`, session.ID, name))

	slog.Debug("broadcasting name update to admins", "sessionID", session.ID, "name", name, "adminCount", len(activeAdmins))

	msg := []byte(payload.String())
	for _, adminConn := range activeAdmins {
		adminConn.WriteMessage(websocket.TextMessage, msg)
	}
}

// HandleAdminChatBan toggles the banned status of a chat session.
func HandleAdminChatBan(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	sessionIDStr := chi.URLParam(kit.Request, "id")
	sessionID, err := strconv.Atoi(sessionIDStr)
	if err != nil {
		return err
	}

	var session models.ChatSession
	if err := db.Get().First(&session, sessionID).Error; err != nil {
		return err
	}

	session.IsBanned = !session.IsBanned
	if err := db.Get().Model(&session).Update("is_banned", session.IsBanned).Error; err != nil {
		return err
	}

	clientsMu.Lock()
	if session.IsBanned {
		if conn, ok := activeClients[session.Identifier]; ok {
			// Push visual ban notice to the live connection
			notice, err := componentToString(kit.Request.Context(), components.ChatBanOOB())
			if err == nil {
				conn.WriteMessage(websocket.TextMessage, []byte(notice))
			}
			conn.Close()
			delete(activeClients, session.Identifier)
		}
	}
	isOnline := activeClients[session.Identifier] != nil || time.Since(session.UpdatedAt) < 15*time.Second
	currentAdminID := getChatIdentifier(kit)

	// Update other admins via WebSocket OOB swap
	oobItemHTML, _ := componentToString(kit.Request.Context(), admin.ChatSessionItem(session, isOnline, templ.Attributes{"hx-swap-oob": "outerHTML"}))

	for aid, adminConn := range activeAdmins {
		if aid == currentAdminID {
			continue
		}
		adminConn.WriteMessage(websocket.TextMessage, []byte(oobItemHTML))
	}
	clientsMu.Unlock()

	return kit.Render(admin.ChatSessionItem(session, isOnline, nil))
}

func broadcastTypingUpdate(ctx context.Context, identifier string, isTyping bool) {
	var session models.ChatSession
	// Find session associated with the client identifier
	if err := db.Get().Where("identifier = ?", identifier).Limit(1).Find(&session).Error; err != nil || session.ID == 0 {
		return
	}

	content := ""
	if isTyping {
		content = "Le client est en train d'écrire..."
	}

	// OOB swap to update the typing indicator in the admin view
	msg := []byte(fmt.Sprintf("<div id=\"typing-indicator-%d\" hx-swap-oob=\"innerHTML\">%s</div>",
		session.ID,
		content,
	))

	clientsMu.Lock()
	defer clientsMu.Unlock()

	for _, adminConn := range activeAdmins {
		adminConn.WriteMessage(websocket.TextMessage, msg)
	}
}
