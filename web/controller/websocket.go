package controller

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/util/common"
	"github.com/mhsanaei/3x-ui/v2/web/middleware"
	"github.com/mhsanaei/3x-ui/v2/web/session"
	"github.com/mhsanaei/3x-ui/v2/web/websocket"

	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

var upgrader = ws.Upgrader{
	ReadBufferSize:    32768,
	WriteBufferSize:   32768,
	EnableCompression: true, // Negotiate permessage-deflate compression if the client supports it

	CheckOrigin: middleware.SameOrigin,
}

// WebSocketController handles WebSocket connections for real-time updates
type WebSocketController struct {
	BaseController
	hub *websocket.Hub
}

// NewWebSocketController creates a new WebSocket controller
func NewWebSocketController(hub *websocket.Hub) *WebSocketController {
	return &WebSocketController{
		hub: hub,
	}
}

// HandleWebSocket handles WebSocket connections
func (w *WebSocketController) HandleWebSocket(c *gin.Context) {
	// Check authentication. The user is captured rather than discarded: the socket
	// is tagged with their id so scoped payloads reach only them.
	user := session.GetLoginUser(c)
	if user == nil || user.NeedsTwoFactorEnrollment() {
		logger.Warningf("Unauthorized WebSocket connection attempt from %s", getRemoteIp(c))
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("Failed to upgrade WebSocket connection:", err)
		return
	}

	// Create client
	clientID := uuid.New().String()
	client := &websocket.Client{
		ID:     clientID,
		UserId: user.Id,
		Hub:    w.hub,
		Send:   make(chan []byte, 512), // Increased from 256 to 512 to prevent overflow
		Topics: make(map[websocket.MessageType]bool),
	}

	// Register client
	w.hub.Register(client)
	logger.Debugf("WebSocket client %s registered from %s", clientID, getRemoteIp(c))

	// Start goroutines for reading and writing
	snapshot := *user
	go w.writePump(client, conn, &snapshot)
	go w.readPump(client, conn)
}

// readPump pumps messages from the WebSocket connection to the hub
func (w *WebSocketController) readPump(client *websocket.Client, conn *ws.Conn) {
	defer func() {
		if r := common.Recover("WebSocket readPump panic"); r != nil {
			logger.Error("WebSocket readPump panic recovered:", r)
		}
		w.hub.Unregister(client)
		conn.Close()
	}()

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	conn.SetReadLimit(maxMessageSize)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if ws.IsUnexpectedCloseError(err, ws.CloseGoingAway, ws.CloseAbnormalClosure) {
				logger.Debugf("WebSocket read error for client %s: %v", client.ID, err)
			}
			break
		}

		// Validate message size
		if len(message) > maxMessageSize {
			logger.Warningf("WebSocket message from client %s exceeds max size: %d bytes", client.ID, len(message))
			continue
		}

		// Handle incoming messages (e.g., subscription requests)
		// For now, we'll just log them
		logger.Debugf("Received WebSocket message from client %s: %s", client.ID, string(message))
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (w *WebSocketController) writePump(client *websocket.Client, conn *ws.Conn, snapshot *model.User) {
	ticker := time.NewTicker(pingPeriod)
	overviewDeniedSent := false
	defer func() {
		if r := common.Recover("WebSocket writePump panic"); r != nil {
			logger.Error("WebSocket writePump panic recovered:", r)
		}
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.Send:
			current := session.CurrentUserSession(snapshot)
			if current == nil {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				conn.WriteMessage(ws.CloseMessage, []byte{})
				return
			}

			message, permitted := filterSocketMessage(current, message)
			if !permitted {
				if !overviewDeniedSent && !overviewAccess(current) {
					notice, _ := json.Marshal(websocket.Message{Type: websocket.MessageTypeOverviewAccess, Payload: gin.H{"allowed": false}, Time: time.Now().UnixMilli()})
					if err := conn.WriteMessage(ws.TextMessage, notice); err != nil {
						return
					}
					overviewDeniedSent = true
				}
				continue
			}
			if overviewDeniedSent {
				var delivered struct {
					Type websocket.MessageType `json:"type"`
				}
				if json.Unmarshal(message, &delivered) == nil && delivered.Type == websocket.MessageTypeStatus {
					overviewDeniedSent = false
				}
			}

			// Send each message individually (no batching)
			// This ensures each JSON message is sent separately and can be parsed correctly
			if err := conn.WriteMessage(ws.TextMessage, message); err != nil {
				logger.Debugf("WebSocket write error for client %s: %v", client.ID, err)
				return
			}

		case <-ticker.C:
			if !session.IsCurrentUserSession(snapshot) {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(ws.PingMessage, nil); err != nil {
				logger.Debugf("WebSocket ping error for client %s: %v", client.ID, err)
				return
			}
		}
	}
}

// Status follows the same live authorization and representative traffic scope
// as REST. Client-specific messages keep their existing per-user delivery.
func filterSocketMessage(user *model.User, message []byte) ([]byte, bool) {
	var envelope struct {
		Type    websocket.MessageType `json:"type"`
		Payload json.RawMessage       `json:"payload"`
		Time    int64                 `json:"time"`
	}
	if json.Unmarshal(message, &envelope) != nil {
		return message, true
	}
	if envelope.Type == websocket.MessageTypeXrayState {
		return message, overviewAccess(user) || (!isRepresentative(user) && (user.Can(model.PermXraySettings) || user.Can(model.PermCoreSettings)))
	}
	if envelope.Type != websocket.MessageTypeStatus {
		return message, true
	}
	if !overviewAccess(user) {
		return nil, false
	}
	if !isRepresentative(user) {
		return message, true
	}
	var status service.Status
	if json.Unmarshal(envelope.Payload, &status) != nil {
		return nil, false
	}
	result, err := json.Marshal(websocket.Message{Type: envelope.Type, Time: envelope.Time, Payload: service.StatusForUser(&status, user)})
	return result, err == nil
}
