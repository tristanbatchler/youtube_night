package websocket

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In production, you should check the origin
		return true
	},
}

// Message types for WebSocket communication
const (
	GameStartMessage    = "game_start"
	PlayerJoinMessage   = "player_join"
	PlayerLeaveMessage  = "player_leave"
	GameStopMessage     = "game_stop"
	VideoChangeMessage  = "video_change"  // New message type for video changes
	CurrentVideoMessage = "current_video" // New message type for informing newcomers
)

// Connection wraps a WebSocket connection
type Connection struct {
	ws   *websocket.Conn
	send chan []byte
}

// ReadPump pumps messages from the WebSocket connection to the hub
func (c *Connection) ReadPump(client *Client) {
	defer func() {
		client.hub.unregister <- client
		c.ws.Close()
	}()

	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				client.hub.logger.Printf("WebSocket read error: %v", err)
			}
			break
		}
		// We're not handling incoming messages from clients currently
		// This could be expanded later for chat or other interactive features
	}
}

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Connection) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel
				c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.ws.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWs handles WebSocket requests from clients
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request, userID int32, gangID int32, isHost bool) {
	// Upgrade the HTTP connection to a WebSocket connection
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		hub.logger.Printf("Error upgrading to WebSocket: %v", err)
		return
	}

	// Create a new client and register it with the hub
	client := &Client{
		GangID: gangID,
		UserID: userID,
		IsHost: isHost,
		Send:   make(chan []byte, 256),
		hub:    hub,
	}

	// Create a new connection
	conn := &Connection{
		ws:   ws,
		send: client.Send,
	}
	client.conn = conn

	// Register the client with the hub
	client.hub.register <- client

	// Start the client's read and write pumps
	go conn.WritePump()
	conn.ReadPump(client)
}

// SendGameStart sends a game start message to all clients in a gang
func SendGameStart(hub *Hub, gangID int32) {
	hub.BroadcastToGang(gangID, []byte(GameStartMessage))
}

// SendGameStop sends a game stop message to all clients in a gang
func SendGameStop(hub *Hub, gangID int32) {
	hub.BroadcastToGang(gangID, []byte(GameStopMessage))
}

// SendCurrentVideo notifies a specific client about the currently playing video
func SendCurrentVideo(hub *Hub, client *Client, videoID string, index int, title string, channel string, timestamp float64) {
	// Create a JSON message with the video details and current timestamp
	message := fmt.Sprintf(`{"type":"%s","videoId":"%s","index":%d,"title":"%s","channel":"%s","timestamp":%f}`,
		CurrentVideoMessage, videoID, index, title, channel, timestamp)

	// Send only to the specific client
	select {
	case client.Send <- []byte(message):
		// Message sent successfully
		hub.logger.Printf("Sent current video info to user %d in gang %d", client.UserID, client.GangID)
	default:
		// Failed to send
		hub.logger.Printf("Failed to send current video info to user %d in gang %d", client.UserID, client.GangID)
	}
}

// SendVideoChange notifies all clients in a gang about a video change
func SendVideoChange(hub *Hub, gangID int32, videoID string, index int, title string, channel string) {
	// Store the current video details for this gang
	hub.SetCurrentVideo(gangID, &CurrentVideo{
		VideoID:   videoID,
		Index:     index,
		Title:     title,
		Channel:   channel,
		StartedAt: time.Now(),
	})

	// Create a JSON message with the video details
	message := fmt.Sprintf(`{"type":"%s","videoId":"%s","index":%d,"title":"%s","channel":"%s"}`,
		VideoChangeMessage, videoID, index, title, channel)
	hub.BroadcastToGang(gangID, []byte(message))
}
