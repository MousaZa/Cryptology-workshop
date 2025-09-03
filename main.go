package main

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/MousaZa/cryptology-workshop/models"
	"github.com/MousaZa/cryptology-workshop/templates"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin
	},
}

type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Println("Client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
			h.mu.Unlock()
			log.Println("Client disconnected")

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case <-time.After(1 * time.Second):
					// Timeout - skip this client
					continue
				default:
					err := client.WriteMessage(websocket.TextMessage, message)
					if err != nil {
						delete(h.clients, client)
						client.Close()
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

func main() {
	r := gin.Default()
	m := models.Messages{}
	hub := newHub()
	go hub.run()

	r.GET("/sender/:id", func(ctx *gin.Context) {
		id := ctx.Param("id")
		t, err := strconv.Atoi(id)
		if err != nil {
			ctx.JSON(400, "Error")
		}
		templates.Sender(t).Render(ctx.Request.Context(), ctx.Writer)
	})

	r.GET("/reciever", func(ctx *gin.Context) {
		templates.Reciever(m).Render(ctx.Request.Context(), ctx.Writer)
	})

	// WebSocket endpoint
	r.GET("/ws", func(ctx *gin.Context) {
		conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
		if err != nil {
			log.Println("WebSocket upgrade error:", err)
			return
		}
		defer conn.Close()

		hub.register <- conn

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				hub.unregister <- conn
				break
			}
		}
	})

	// Message submission endpoint
	r.POST("/send-message", func(ctx *gin.Context) {
		teamID := ctx.PostForm("team")
		message := ctx.PostForm("message")

		if teamID == "" {
			ctx.JSON(400, gin.H{"error": "Team ID is required"})
			return
		}

		// Update the messages based on team ID (allow empty messages for clearing)
		switch teamID {
		case "1":
			m.Team1 = message
		case "2":
			m.Team2 = message
		case "3":
			m.Team3 = message
		case "4":
			m.Team4 = message
		default:
			ctx.JSON(400, gin.H{"error": "Invalid team ID"})
			return
		}

		// Broadcast update to all WebSocket clients
		hub.broadcast <- []byte("update")

		// Return status indicator for HTMX
		ctx.Header("Content-Type", "text/html")
		if message == "" {
			ctx.String(200, `<div class="p-2 rounded-lg bg-gray-50 border border-gray-200 text-gray-600 text-sm">Message cleared</div>`)
		} else {
			ctx.String(200, `<div class="p-2 rounded-lg bg-green-50 border border-green-200 text-green-600 text-sm">Message updated in real-time</div>`)
		}
	})

	// Endpoint for HTMX to fetch updated messages
	r.GET("/messages", func(ctx *gin.Context) {
		ctx.Header("Content-Type", "text/html")
		templates.MessagesPartial(m).Render(ctx.Request.Context(), ctx.Writer)
	})

	r.Run(":8000")
}
