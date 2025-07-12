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
				// Calculate how long the video has been playing
				var elapsedTime float64
				if currentVideo.IsPaused {
					// If the video is paused, use the timestamp where it was paused
					elapsedTime = currentVideo.PausedAt
					h.logger.Printf("Video is paused, using PausedAt timestamp: %.2f", elapsedTime)
				} else {
					// Calculate elapsed time correctly accounting for paused periods
					rawElapsed := time.Since(currentVideo.StartedAt).Seconds()
					// Subtract the total paused time from the raw elapsed time
					elapsedTime = rawElapsed - currentVideo.TotalPausedTime

					if elapsedTime < 0 {
						// Safety check to prevent negative timestamps
						h.logger.Printf("Warning: Calculated negative timestamp (%.2f), resetting to 0", elapsedTime)
						elapsedTime = 0
					}

					h.logger.Printf("Video is playing, calculated timestamp: %.2f (raw: %.2f, paused: %.2f)",
						elapsedTime, rawElapsed, currentVideo.TotalPausedTime)
				}

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
		video.LastPause = video.StartedAt
	}

	// Initialize TotalPausedTime to 0 for a new video
	video.TotalPausedTime = 0

	h.currentVideos[gangID] = video
	h.mu.Unlock()

	h.logger.Printf("Current video set for gang %d: %s (index: %d, timestamp: 0.0)",
		gangID, video.VideoID, video.Index)
}

// UpdatePlaybackState updates the playback state (paused/playing) for a gang
func (h *Hub) UpdatePlaybackState(gangID int32, isPaused bool, timestamp float64) {
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

	// Handle state change
	if isPaused && !video.IsPaused {
		// Transitioning from playing to paused
		video.IsPaused = true
		video.PausedAt = timestamp
		video.LastPause = time.Now()
		h.logger.Printf("Video paused for gang %d at timestamp %.2f", gangID, timestamp)
	} else if !isPaused && video.IsPaused {
		// Transitioning from paused to playing
		video.IsPaused = false

		// Calculate how long this pause lasted
		pauseDuration := time.Since(video.LastPause).Seconds()
		// Add it to the total paused time counter
		video.TotalPausedTime += pauseDuration

		h.logger.Printf("Video resumed for gang %d from timestamp %.2f (pause duration: %.2f, total paused: %.2f)",
			gangID, timestamp, pauseDuration, video.TotalPausedTime)
	}
}
