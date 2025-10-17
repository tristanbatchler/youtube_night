package websocket

import (
	"log"
	"sync"
	"time"
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

// CurrentVideo represents the currently playing video for a gang
type CurrentVideo struct {
	VideoID         string
	Index           int
	Title           string
	Channel         string
	StartedAt       time.Time
	IsPaused        bool
	PausedAt        float64   // Timestamp where video was paused
	LastPause       time.Time // Time when the most recent pause occurred
	TotalPausedTime float64   // Accumulated time in seconds the video has been paused
	HostTimestamp   float64   // Host-reported playback position when UpdatedAt was recorded
	UpdatedAt       time.Time // Last time the host reported playback state
	LastAction      string    // Last host action (play, pause, seek)
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	// Registered clients by gang ID
	gangClients map[int32]map[*Client]bool

	// Current video playing for each gang
	currentVideos map[int32]*CurrentVideo

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
		gangClients:   make(map[int32]map[*Client]bool),
		currentVideos: make(map[int32]*CurrentVideo),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		logger:        logger,
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

			// Check if there's a video already playing in this gang
			if currentVideo, exists := h.currentVideos[client.GangID]; exists {
				// Calculate the host-aligned timestamp that late joiners should start from
				elapsedTime := currentVideo.HostTimestamp
				if !currentVideo.IsPaused {
					timeSinceUpdate := time.Since(currentVideo.UpdatedAt).Seconds()
					elapsedTime += timeSinceUpdate
				}
				if elapsedTime < 0 {
					// Safety check to prevent negative timestamps
					h.logger.Printf("Warning: Calculated negative timestamp (%.2f), resetting to 0", elapsedTime)
					elapsedTime = 0
				}
				h.logger.Printf("Late joiner sync -> action: %s, paused: %t, base: %.2f, delta: %.2f, start: %.2f",
					currentVideo.LastAction, currentVideo.IsPaused, currentVideo.HostTimestamp,
					time.Since(currentVideo.UpdatedAt).Seconds(), elapsedTime)

				// Use a goroutine to avoid blocking the hub's main loop
				go func(c *Client, cv *CurrentVideo, timestamp float64) {
					SendCurrentVideo(h, c, cv.VideoID, cv.Index, cv.Title, cv.Channel, timestamp)
				}(client, currentVideo, elapsedTime)
			} else {
				h.logger.Printf("No current video for gang %d, user %d connected", client.GangID, client.UserID)
			}
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

// SetCurrentVideo updates the current video for a gang
func (h *Hub) SetCurrentVideo(gangID int32, video *CurrentVideo) {
	h.mu.Lock()
	if h.currentVideos == nil {
		h.currentVideos = make(map[int32]*CurrentVideo)
	}

	// Ensure LastPause is initialized
	if video.LastPause.IsZero() {
		video.LastPause = time.Time{}
	}

	now := time.Now()
	video.StartedAt = now
	video.TotalPausedTime = 0
	video.IsPaused = false
	video.PausedAt = 0
	video.HostTimestamp = 0
	video.UpdatedAt = now
	video.LastAction = "play"

	h.currentVideos[gangID] = video
	h.mu.Unlock()

	h.logger.Printf("Current video set for gang %d: %s (index: %d, timestamp: 0.0)",
		gangID, video.VideoID, video.Index)
}

// UpdatePlaybackState updates the playback state (paused/playing) for a gang
func (h *Hub) UpdatePlaybackState(gangID int32, action string, timestamp float64, isPaused bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.currentVideos == nil {
		h.currentVideos = make(map[int32]*CurrentVideo)
	}

	video, exists := h.currentVideos[gangID]
	if !exists {
		h.logger.Printf("Cannot update playback state - no video exists for gang %d", gangID)
		return
	}

	now := time.Now()
	wasPaused := video.IsPaused

	if !wasPaused && isPaused {
		video.LastPause = now
	}

	if wasPaused && !isPaused && !video.LastPause.IsZero() {
		pauseDuration := now.Sub(video.LastPause).Seconds()
		if pauseDuration < 0 {
			pauseDuration = 0
		}
		video.TotalPausedTime += pauseDuration
		video.LastPause = time.Time{}
	}

	video.IsPaused = isPaused
	if isPaused {
		video.PausedAt = timestamp
	} else {
		video.PausedAt = 0
	}

	video.HostTimestamp = timestamp
	video.UpdatedAt = now
	video.LastAction = action

	h.logger.Printf("Playback update for gang %d -> action: %s, paused: %t, timestamp: %.2f", gangID, action, isPaused, timestamp)
}
