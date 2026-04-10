// Package gateway implements a WebSocket channel for the gateway mode.
package gateway

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"dogclaw/pkg/channel"
	"github.com/gorilla/websocket"
)

// MessageType defines the type of WebSocket message
type MessageType string

const (
	// MessageTypeText is a regular text message
	MessageTypeText MessageType = "text"
	// MessageTypeError is an error message
	MessageTypeError MessageType = "error"
	// MessageTypePing is a ping message
	MessageTypePing MessageType = "ping"
	// MessageTypePong is a pong message
	MessageTypePong MessageType = "pong"
)

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type    MessageType `json:"type"`
	Content string      `json:"content"`
}

// Channel implements a WebSocket channel
type Channel struct {
	port        int
	upgrader    websocket.Upgrader
	server      *http.Server
	connections map[*websocket.Conn]bool
	mu          sync.RWMutex
	factory     channel.EngineFactory
	ctx         context.Context
	cancel      context.CancelFunc
}

// Config holds the gateway channel configuration
type Config struct {
	Port int
}

// NewChannel creates a new gateway WebSocket channel
func NewChannel(cfg Config) *Channel {
	ctx, cancel := context.WithCancel(context.Background())
	return &Channel{
		port: cfg.Port,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
		connections: make(map[*websocket.Conn]bool),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (c *Channel) Info() channel.Info {
	return channel.Info{Name: "gateway"}
}

func (c *Channel) Start(ctx context.Context, factory channel.EngineFactory) error {
	c.factory = factory

	mux := http.NewServeMux()
	mux.HandleFunc("/chat", c.handleWebSocket)
	mux.HandleFunc("/", c.handleIndex)

	c.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", c.port),
		Handler: mux,
	}

	fmt.Printf("🌐 Gateway WebSocket server starting on port %d...\n", c.port)
	fmt.Printf("📝 Test page available at: http://localhost:%d/\n", c.port)

	go func() {
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("❌ Gateway server error: %v\n", err)
		}
	}()

	return nil
}

func (c *Channel) Stop() {
	c.cancel()

	// Close all WebSocket connections
	c.mu.Lock()
	for conn := range c.connections {
		conn.Close()
		delete(c.connections, conn)
	}
	c.mu.Unlock()

	// Shutdown HTTP server
	if c.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5)
		defer cancel()
		c.server.Shutdown(ctx)
	}
}

func (c *Channel) Send(ctx context.Context, chatID, message string) error {
	// Broadcast to all connections
	c.mu.RLock()
	defer c.mu.RUnlock()

	for conn := range c.connections {
		msg := WebSocketMessage{
			Type:    MessageTypeText,
			Content: message,
		}
		if err := conn.WriteJSON(msg); err != nil {
			fmt.Printf("❌ Failed to send message: %v\n", err)
			conn.Close()
			delete(c.connections, conn)
		}
	}
	return nil
}

func (c *Channel) handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>DogClaw Gateway Chat</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            max-width: 800px;
            margin: 0 auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .chat-container {
            background-color: white;
            border-radius: 12px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            overflow: hidden;
        }
        .chat-header {
            background-color: #4a90d9;
            color: white;
            padding: 20px;
            text-align: center;
        }
        .chat-messages {
            height: 400px;
            overflow-y: auto;
            padding: 20px;
        }
        .message {
            margin-bottom: 15px;
            padding: 10px 15px;
            border-radius: 18px;
            max-width: 70%;
        }
        .message.user {
            background-color: #4a90d9;
            color: white;
            margin-left: auto;
        }
        .message.assistant {
            background-color: #e8e8e8;
            color: #333;
        }
        .chat-input {
            display: flex;
            padding: 20px;
            border-top: 1px solid #eee;
        }
        .chat-input input {
            flex: 1;
            padding: 12px 15px;
            border: 1px solid #ddd;
            border-radius: 24px;
            font-size: 16px;
        }
        .chat-input button {
            margin-left: 10px;
            padding: 12px 25px;
            background-color: #4a90d9;
            color: white;
            border: none;
            border-radius: 24px;
            cursor: pointer;
            font-size: 16px;
        }
        .chat-input button:hover {
            background-color: #3a7bc8;
        }
        .status {
            text-align: center;
            padding: 10px;
            font-size: 14px;
        }
        .status.connected {
            color: #4CAF50;
        }
        .status.disconnected {
            color: #f44336;
        }
    </style>
</head>
<body>
    <div class="chat-container">
        <div class="chat-header">
            <h1>🐕 DogClaw Chat</h1>
        </div>
        <div id="status" class="status disconnected">未连接</div>
        <div id="messages" class="chat-messages"></div>
        <div class="chat-input">
            <input type="text" id="messageInput" placeholder="输入消息..." disabled>
            <button id="sendButton" onclick="sendMessage()" disabled>发送</button>
        </div>
    </div>

    <script>
        let ws;
        const messageInput = document.getElementById('messageInput');
        const sendButton = document.getElementById('sendButton');
        const messagesDiv = document.getElementById('messages');
        const statusDiv = document.getElementById('status');

        function connect() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = protocol + '//' + window.location.host + '/chat';
            
            ws = new WebSocket(wsUrl);

            ws.onopen = function() {
                statusDiv.textContent = '已连接';
                statusDiv.className = 'status connected';
                messageInput.disabled = false;
                sendButton.disabled = false;
                messageInput.focus();
            };

            ws.onclose = function() {
                statusDiv.textContent = '连接已断开，正在重连...';
                statusDiv.className = 'status disconnected';
                messageInput.disabled = true;
                sendButton.disabled = true;
                setTimeout(connect, 3000);
            };

            ws.onerror = function(error) {
                console.error('WebSocket error:', error);
            };

            ws.onmessage = function(event) {
                const data = JSON.parse(event.data);
                if (data.type === 'text') {
                    addMessage(data.content, 'assistant');
                } else if (data.type === 'error') {
                    addMessage('错误: ' + data.content, 'assistant');
                }
            };
        }

        function sendMessage() {
            const message = messageInput.value.trim();
            if (message && ws.readyState === WebSocket.OPEN) {
                const msg = {
                    type: 'text',
                    content: message
                };
                ws.send(JSON.stringify(msg));
                addMessage(message, 'user');
                messageInput.value = '';
                messageInput.focus();
            }
        }

        function addMessage(content, type) {
            const messageDiv = document.createElement('div');
            messageDiv.className = 'message ' + type;
            messageDiv.textContent = content;
            messagesDiv.appendChild(messageDiv);
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        }

        // Handle Enter key
        messageInput.addEventListener('keypress', function(e) {
            if (e.key === 'Enter') {
                sendMessage();
            }
        });

        // Connect on page load
        connect();
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func (c *Channel) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("❌ WebSocket upgrade error: %v\n", err)
		return
	}
	defer conn.Close()

	// Register connection
	c.mu.Lock()
	c.connections[conn] = true
	c.mu.Unlock()

	fmt.Printf("🔌 New WebSocket connection established\n")

	// Create query engine for this connection
	qe := c.factory("gateway")
	qe.AutoResumeLatestSession(c.ctx)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			var msg WebSocketMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					fmt.Printf("❌ WebSocket error: %v\n", err)
				}
				c.mu.Lock()
				delete(c.connections, conn)
				c.mu.Unlock()
				return
			}

			switch msg.Type {
			case MessageTypePing:
				// Respond with pong
				pongMsg := WebSocketMessage{
					Type:    MessageTypePong,
					Content: "",
				}
				conn.WriteJSON(pongMsg)
			case MessageTypeText:
				// Process the message
				if err := qe.SubmitMessage(c.ctx, msg.Content); err != nil {
					// Send error message
					errMsg := WebSocketMessage{
						Type:    MessageTypeError,
						Content: err.Error(),
					}
					conn.WriteJSON(errMsg)
					continue
				}

				// Get and send response
				response := qe.GetLastAssistantText()
				if response != "" {
					respMsg := WebSocketMessage{
						Type:    MessageTypeText,
						Content: response,
					}
					conn.WriteJSON(respMsg)
				}
			}
		}
	}
}

// Assert Channel implements channel.Interface and channel.Sender
var _ channel.Interface = (*Channel)(nil)
var _ channel.Sender = (*Channel)(nil)
