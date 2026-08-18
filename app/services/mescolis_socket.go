package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const mescolisSocketURL = "wss://api.mescolis.tn:4001/socket.io/?EIO=4&transport=websocket"

// MescolisEvent is the payload received on the "mescolis-events" channel.
type MescolisEvent struct {
	Barcode               string `json:"barcode"`
	Status                string `json:"status"`
	UpdatedAt             string `json:"updated_at"`
	DeliverymanName       string `json:"deliveryman_name,omitempty"`
	DeliverymanPhoneNumber string `json:"deliveryman_phone_number,omitempty"`
	Qualification         string `json:"qualification,omitempty"`
	DeliveredAt           string `json:"delivered_at,omitempty"`
	ReceiverName          string `json:"receiver_name,omitempty"`
}

// MescolisSocket manages a persistent Socket.IO connection to Mes Colis
// for real-time parcel status updates.
type MescolisSocket struct {
	apiKey   string
	conn     *websocket.Conn
	mu       sync.Mutex
	closeCh  chan struct{}
	onEvent  func(MescolisEvent)
}

// NewMescolisSocket creates a new socket client. Call Start() to connect.
func NewMescolisSocket(apiKey string, onEvent func(MescolisEvent)) *MescolisSocket {
	return &MescolisSocket{
		apiKey:  apiKey,
		onEvent: onEvent,
		closeCh: make(chan struct{}),
	}
}

// Start connects to the Mes Colis socket server and listens for events.
// It reconnects automatically on disconnection. Blocks until Stop() is called.
func (s *MescolisSocket) Start() {
	for {
		select {
		case <-s.closeCh:
			return
		default:
		}
		if err := s.connect(); err != nil {
			slog.Error("mescolis socket: connection failed, retrying in 5s", "err", err)
			time.Sleep(5 * time.Second)
		}
	}
}

// Stop closes the connection and stops the listener.
func (s *MescolisSocket) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	close(s.closeCh)
	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *MescolisSocket) connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(mescolisSocketURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()

	slog.Info("mescolis socket: connected")
	defer func() {
		conn.Close()
		slog.Info("mescolis socket: disconnected")
	}()

	// Read Engine.IO open packet: 0{"sid":"...","pingInterval":...,"pingTimeout":...}
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read open packet: %w", err)
	}
	var openMsg string
	var pingInterval time.Duration
	if err := parseEngineOpen(msg, &openMsg, &pingInterval); err != nil {
		return fmt.Errorf("parse open packet: %w", err)
	}
	slog.Info("mescolis socket: engine open", "ping_interval", pingInterval)

	// Send Socket.IO CONNECT with auth: 40{"token":"..."}
	connectPkt := fmt.Sprintf(`40{"token":"%s"}`, s.apiKey)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(connectPkt)); err != nil {
		return fmt.Errorf("send connect: %w", err)
	}

	// Start ping ticker
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	go func() {
		for {
			select {
			case <-pingTicker.C:
				s.mu.Lock()
				err := conn.WriteMessage(websocket.TextMessage, []byte("2"))
				s.mu.Unlock()
				if err != nil {
					slog.Error("mescolis socket: ping failed", "err", err)
					conn.Close()
					return
				}
			case <-s.closeCh:
				return
			}
		}
	}()

	// Read loop
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}
		s.handleMessage(msg)
	}
}

func (s *MescolisSocket) handleMessage(raw []byte) {
	text := string(raw)

	// Engine.IO pong - ignore
	if text == "3" {
		return
	}

	// Engine.IO ping - respond with pong
	if text == "2" {
		s.mu.Lock()
		err := s.conn.WriteMessage(websocket.TextMessage, []byte("3"))
		s.mu.Unlock()
		if err != nil {
			slog.Error("mescolis socket: pong failed", "err", err)
		}
		return
	}

	// Socket.IO CONNECT ack: 40{...}
	if strings.HasPrefix(text, "40") && !strings.HasPrefix(text, "42") {
		slog.Info("mescolis socket: connected to channel", "payload", text[2:])
		return
	}

	// Socket.IO EVENT: 42["mescolis-events",{...}]
	if strings.HasPrefix(text, "42") {
		payload := text[2:]
		var event []json.RawMessage
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			slog.Error("mescolis socket: failed to parse event", "err", err, "payload", payload)
			return
		}
		if len(event) < 2 {
			return
		}

		var channel string
		if err := json.Unmarshal(event[0], &channel); err != nil {
			return
		}

		if channel == "mescolis-events" {
			var evt MescolisEvent
			if err := json.Unmarshal(event[1], &evt); err != nil {
				slog.Error("mescolis socket: failed to parse mescolis-event", "err", err)
				return
			}
			slog.Info("mescolis socket: parcel status update",
				"barcode", evt.Barcode,
				"status", evt.Status,
				"updated_at", evt.UpdatedAt,
			)
			if s.onEvent != nil {
				s.onEvent(evt)
			}
		}
		return
	}

	// Socket.IO DISCONNECT: 41
	if strings.HasPrefix(text, "41") {
		slog.Warn("mescolis socket: server disconnected us")
		return
	}

	slog.Debug("mescolis socket: unhandled message", "msg", text)
}

// parseEngineOpen parses: 0{"sid":"...","pingInterval":25000,...}
func parseEngineOpen(raw []byte, sid *string, pingInterval *time.Duration) error {
	text := string(raw)
	if !strings.HasPrefix(text, "0") {
		return fmt.Errorf("expected open packet (0 prefix), got: %s", text)
	}
	var resp struct {
		SID            string `json:"sid"`
		PingInterval   int    `json:"pingInterval"`
		PingTimeout    int    `json:"pingTimeout"`
		MaxPayload     int    `json:"maxPayload"`
	}
	if err := json.Unmarshal([]byte(text[1:]), &resp); err != nil {
		return fmt.Errorf("parse json: %w", err)
	}
	*sid = resp.SID
	*pingInterval = time.Duration(resp.PingInterval) * time.Millisecond
	return nil
}
