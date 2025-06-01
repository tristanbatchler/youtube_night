package websocket

import (
	"log"
	"sync"
)

// Client represents a WebSocket client connection
type Client struct {
	GangID int32
	UserID int32
	IsHost bool
	Send   chan []byte
	hub    *Hub
	conn   *Connection
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	// Registered clients by gang ID
	gangClients map[int32]map[*Client]bool

	// Register requests
	register chan *Client

	// Unregister requests
	unregister chan *Client

	// Mutex for thread-safe access to the gangClients map
	mu sync.RWMutex

	// Logger
	logger *log.Logger
}

// NewHub creates a new Hub
func NewHub(logger *log.Logger) *Hub {
	return &Hub{
		gangClients: make(map[int32]map[*Client]bool),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		logger:      logger,
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			// Initialize the gang's client map if it doesn't exist
			if _, ok := h.gangClients[client.GangID]; !ok {
				h.gangClients[client.GangID] = make(map[*Client]bool)
			}
			h.gangClients[client.GangID][client] = true
			h.logger.Printf("Client registered: user %d in gang %d (host: %t), total clients in gang: %d",
				client.UserID, client.GangID, client.IsHost, len(h.gangClients[client.GangID]))
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			// Remove the client if it exists
			if _, ok := h.gangClients[client.GangID]; ok {
				if _, ok := h.gangClients[client.GangID][client]; ok {
					delete(h.gangClients[client.GangID], client)
					close(client.Send)
					h.logger.Printf("Client unregistered: user %d in gang %d, remaining clients: %d",
						client.UserID, client.GangID, len(h.gangClients[client.GangID]))

					// Clean up empty gang maps
					if len(h.gangClients[client.GangID]) == 0 {
						delete(h.gangClients, client.GangID)
						h.logger.Printf("Removed empty gang %d from hub", client.GangID)
					}
				}
			}
			h.mu.Unlock()
		}
	}
}

// BroadcastToGang sends a message to all clients in a specific gang
func (h *Hub) BroadcastToGang(gangID int32, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.gangClients[gangID]; ok {
		for client := range clients {
			select {
			case client.Send <- message:
				// Message sent successfully
			default:
				// Failed to send, clean up
				close(client.Send)
				h.mu.RUnlock()
				h.mu.Lock()
				delete(h.gangClients[gangID], client)
				if len(h.gangClients[gangID]) == 0 {
					delete(h.gangClients, gangID)
				}
				h.mu.Unlock()
				h.mu.RLock()
			}
		}
		h.logger.Printf("Broadcast message to %d clients in gang %d", len(clients), gangID)
	} else {
		h.logger.Printf("No clients found in gang %d for broadcast", gangID)
	}
}

// GetConnectedClientsCountByGang returns the number of connected clients for a specific gang
func (h *Hub) GetConnectedClientsCountByGang(gangID int32) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.gangClients[gangID]; ok {
		return len(clients)
	}
	return 0
}

// GetHostClientForGang returns the host client for a specific gang if available
func (h *Hub) GetHostClientForGang(gangID int32) *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.gangClients[gangID]; ok {
		for client := range clients {
			if client.IsHost {
				return client
			}
		}
	}
	return nil
}
