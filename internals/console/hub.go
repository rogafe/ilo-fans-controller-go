package console

import (
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
)

type Event struct {
	Type      string `json:"type"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type clientConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*websocket.Conn]*clientConn
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[*websocket.Conn]*clientConn),
	}
}

func (h *Hub) Register(clientID string, conn *websocket.Conn) {
	if clientID == "" || conn == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[clientID] == nil {
		h.clients[clientID] = make(map[*websocket.Conn]*clientConn)
	}

	h.clients[clientID][conn] = &clientConn{conn: conn}
}

func (h *Hub) Unregister(clientID string, conn *websocket.Conn) {
	if clientID == "" || conn == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	connections := h.clients[clientID]
	if connections == nil {
		return
	}

	delete(connections, conn)
	if len(connections) == 0 {
		delete(h.clients, clientID)
	}
}

func (h *Hub) Send(clientID, eventType, message string) {
	if clientID == "" {
		return
	}

	event := Event{
		Type:      eventType,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	h.mu.RLock()
	connections := h.clients[clientID]
	wrappers := make([]*clientConn, 0, len(connections))
	for _, wrapper := range connections {
		wrappers = append(wrappers, wrapper)
	}
	h.mu.RUnlock()

	for _, wrapper := range wrappers {
		wrapper.mu.Lock()
		writeErr := wrapper.conn.WriteJSON(event)
		wrapper.mu.Unlock()
		if writeErr != nil {
			h.Unregister(clientID, wrapper.conn)
			_ = wrapper.conn.Close()
		}
	}
}
