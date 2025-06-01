package websocket

import (
	"encoding/json"
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
	// Message types
	GameStartMessage   = "game_start"
	PlayerJoinMessage  = "player_join"
	PlayerLeaveMessage = "player_leave"
	GameUpdateMessage  = "game_update"
)

// Message is the structure of messages sent through WebSockets
type Message struct {
	Type    string      `json:"type"`
	Content interface{} `json:"content"`
}

// GameStartContent contains data sent when a game starts
type GameStartContent struct {
	Videos []VideoInfo `json:"videos"`
}

// VideoInfo contains simplified video information
type VideoInfo struct {
	VideoID      string `json:"videoId"`
	Title        string `json:"title"`
	ThumbnailURL string `json:"thumbnailUrl"`
	ChannelName  string `json:"channelName"`
	Description  string `json:"description,omitempty"`
}

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

// SendGameStart broadcasts a game start message with all videos to a specific gang
func SendGameStart(hub *Hub, gangID int32, videos []VideoInfo) {
	message := Message{
		Type: GameStartMessage,
		Content: GameStartContent{
			Videos: videos,
		},
	}

	// Convert message to JSON
	jsonMessage, err := json.Marshal(message)
	if err != nil {
		hub.logger.Printf("Error marshaling game start message: %v", err)
		return
	}

	// Broadcast to the gang
	hub.BroadcastToGang(gangID, jsonMessage)
}
