package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/handlers"
	"github.com/gorilla/websocket"
)

type Vote struct {
	Voter string  `json:"voter"`
	Score *string `json:"score"`
}

type Event struct {
	Type string          `json:"type"`
	Data map[string]Vote `json:"data"`
}

var (
	allowedOrigins = []string{"http://localhost:3000", ""}
	voters         = make(map[string]Vote)
	mutex          sync.Mutex
	clients        = make(map[*websocket.Conn]bool)
	broadcast      = make(chan string)
	upgrader       = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")

			for _, allowedOrigin := range allowedOrigins {
				if origin == allowedOrigin {
					return true
				}
			}
			return false
		}}
)

func eventHandler(w http.ResponseWriter, r *http.Request) {
	var vote Vote
	err := json.NewDecoder(r.Body).Decode(&vote)
	if err != nil || vote.Voter == "" {
		http.Error(w, "Invalid vote data", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	voters[vote.Voter] = Vote{
		Voter: vote.Voter,
		Score: vote.Score,
	}

	event := Event{
		Type: "update",
		Data: voters,
	}

	eventBytes, err := json.Marshal(event)
	if err != nil {
		fmt.Println("Error marshaling voters to JSON:", err)
		return
	}

	broadcast <- string(eventBytes)

	w.WriteHeader(http.StatusOK)
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()

	mutex.Lock()
	clients[ws] = true
	mutex.Unlock()

	for {
		_, message, err := ws.ReadMessage()
		broadcast <- string(message)
		if err != nil {
			mutex.Lock()
			delete(clients, ws)
			mutex.Unlock()
			break
		}
	}
}

func handleMessages() {
	for {
		update := <-broadcast

		mutex.Lock()
		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, []byte(update))
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
		mutex.Unlock()
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleConnections)
	mux.HandleFunc("/event", eventHandler)

	go handleMessages()

	fmt.Println("Server starting...")
	corsOptions := handlers.AllowedOrigins(allowedOrigins)
	corsMethods := handlers.AllowedMethods([]string{"GET", "POST", "OPTIONS"})
	corsHeaders := handlers.AllowedHeaders([]string{"Content-Type"})
	log.Fatal(http.ListenAndServe(":80", handlers.CORS(corsOptions, corsMethods, corsHeaders)(mux)))
}
