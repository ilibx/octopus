package kanban

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/logger"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now - should be configured in production
		return true
	},
}

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
}

// WSClient represents a connected WebSocket client
type WSClient struct {
	conn   *websocket.Conn
	send   chan []byte
	hub    *WSHub
	mu     sync.Mutex
	closed bool
}

// WSHub manages all WebSocket connections
type WSHub struct {
	clients    map[*WSClient]bool
	register   chan *WSClient
	unregister chan *WSClient
	broadcast  chan []byte
	mu         sync.RWMutex
	msgBus     *bus.MessageBus
	subId      string
}

// NewWSHub creates a new WebSocket hub
func NewWSHub(msgBus *bus.MessageBus) *WSHub {
	hub := &WSHub{
		clients:    make(map[*WSClient]bool),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
		broadcast:  make(chan []byte, 256),
		msgBus:     msgBus,
	}

	// Subscribe to kanban events
	hub.subscribeToEvents()

	return hub
}

// subscribeToEvents subscribes to relevant kanban events
func (h *WSHub) subscribeToEvents() {
	h.subId = h.msgBus.Subscribe("task.", func(event *bus.Event) {
		msg := WSMessage{
			Type:      event.Type,
			Payload:   event.Data,
			Timestamp: time.Now(),
		}

		data, err := json.Marshal(msg)
		if err != nil {
			logger.ErrorCF("websocket", "Failed to marshal event", map[string]any{"error": err.Error()})
			return
		}

		h.broadcast <- data
	})
}

// Run starts the hub's main loop
func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			logger.InfoCF("websocket", "Client connected", map[string]any{"client_count": len(h.clients)})

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			logger.InfoCF("websocket", "Client disconnected", map[string]any{"client_count": len(h.clients)})

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client buffer full, disconnect
					go client.close()
				}
			}
			h.mu.RUnlock()
		}
	}
}

// HandleWebSocket handles WebSocket upgrade requests
func (h *WSHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.ErrorCF("websocket", "WebSocket upgrade failed", map[string]any{"error": err.Error()})
		return
	}

	client := &WSClient{
		conn:   conn,
		send:   make(chan []byte, 256),
		hub:    h,
		closed: false,
	}

	h.register <- client

	// Start writer goroutine
	go client.writePump()

	// Start reader goroutine
	go client.readPump()
}

// Broadcast sends a message to all connected clients
func (h *WSHub) Broadcast(messageType string, payload interface{}) {
	msg := WSMessage{
		Type:      messageType,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logger.ErrorCF("websocket", "Failed to marshal broadcast message", map[string]any{"error": err.Error()})
		return
	}

	h.broadcast <- data
}

// Close shuts down the hub and all connections
func (h *WSHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.subId != "" {
		h.msgBus.Unsubscribe(h.subId)
	}

	for client := range h.clients {
		client.close()
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *WSClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				return
			}

			c.mu.Lock()
			err := c.conn.WriteMessage(websocket.TextMessage, message)
			c.mu.Unlock()

			if err != nil {
				logger.ErrorCF("websocket", "Failed to write message", map[string]any{"error": err.Error()})
				c.close()
				return
			}

		case <-ticker.C:
			c.mu.Lock()
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			c.mu.Unlock()

			if err != nil {
				logger.ErrorCF("websocket", "Failed to send ping", map[string]any{"error": err.Error()})
				c.close()
				return
			}
		}
	}
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *WSClient) readPump() {
	defer func() {
		c.hub.unregister <- c
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.ErrorCF("websocket", "WebSocket error", map[string]any{"error": err.Error()})
			}
			break
		}

		// Handle incoming messages if needed
		logger.DebugCF("websocket", "Received message", map[string]any{"message": string(message)})
	}
}

// close closes the WebSocket connection
func (c *WSClient) close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	c.conn.Close()
}
