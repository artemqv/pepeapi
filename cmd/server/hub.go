package main

import "github.com/gorilla/websocket"

type BroadcastMessage struct {
	Room    int    `json:"room"`
	Message string `json:"message"`
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
	room int
}

type Hub struct {
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	broadcast  chan BroadcastMessage
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan BroadcastMessage),
	}
}

func (h *Hub) Run() {
	for {
		select {

		case client := <-h.register:
			h.clients[client] = struct{}{}

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}

		case message := <-h.broadcast:
			data := []byte(message.Message)
			for client := range h.clients {
				if client.room != message.Room {
					continue
				}
				select {
				case client.send <- data:
				default:
					delete(h.clients, client)
					close(client.send)
					client.conn.Close()
				}

			}
		}
	}
}
