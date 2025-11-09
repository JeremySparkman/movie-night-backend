package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"slices"

	"github.com/gorilla/handlers"
	"github.com/gorilla/websocket"
)

type Vote struct {
	Room  string  `json:"room"`
	Score *string `json:"score"`
	Voter string  `json:"voter"`
}

type Event struct {
	Data Room   `json:"data"`
	Type string `json:"type"`
}

type Room struct {
	Voters map[string]Vote `json:"voters"`
}

var (
	allowedOrigins = []string{"http://localhost:3000", "https://movie-knights-inky.vercel.app"}
	broadcast      = make(chan string)
	clients        = make(map[*websocket.Conn]bool)
	mutex          sync.Mutex
	rooms          = make(map[string]Room)
	upgrader       = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")

			return slices.Contains(allowedOrigins, origin)
		}}
)

func eventHandler(w http.ResponseWriter, r *http.Request) {
	var vote Vote
	err := json.NewDecoder(r.Body).Decode(&vote)

	if err != nil || vote.Voter == "" {
		fmt.Println(vote)
		http.Error(w, "Invalid vote data", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	room, exists := rooms[vote.Room]
	if !exists {
		room = Room{Voters: make(map[string]Vote)}
	}
	room.Voters[vote.Voter] = Vote{
		Voter: vote.Voter,
		Score: vote.Score,
		Room:  vote.Room,
	}
	rooms[vote.Room] = room

	eventBytes, err := json.Marshal(rooms)

	if err != nil {
		fmt.Println("Error marshaling voters to JSON:", err)
		return
	}

	broadcast <- string(eventBytes)

	w.WriteHeader(http.StatusOK)
}

func roomsHandler(w http.ResponseWriter, r *http.Request) {
	mutex.Lock()
	names := make([]string, 0, len(rooms))
	for name := range rooms {
		names = append(names, name)
	}
	mutex.Unlock()

	response := struct {
		Type  string   `json:"type"`
		Rooms []string `json:"rooms"`
	}{
		Type:  "rooms",
		Rooms: names,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	var room string
	err := json.NewDecoder(r.Body).Decode(&room)

	if err != nil || room == "" {
		http.Error(w, "Invalid room data", http.StatusBadRequest)
		return
	}

	event := Event{
		Type: "status",
		Data: rooms[room],
	}

	statusBytes, err := json.Marshal(event)
	if err != nil {
		fmt.Println("Error marshaling status to JSON:", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(statusBytes)
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
	mux.HandleFunc("/rooms", roomsHandler)
	mux.HandleFunc("/status", statusHandler)

	go handleMessages()

	fmt.Println("Server starting...")
	corsOptions := handlers.AllowedOrigins(allowedOrigins)
	corsMethods := handlers.AllowedMethods([]string{"GET", "POST", "OPTIONS"})
	corsHeaders := handlers.AllowedHeaders([]string{"Content-Type"})
	corsCredentials := handlers.AllowCredentials()

	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}
	log.Fatal(http.ListenAndServe(":"+port, handlers.CORS(corsOptions, corsMethods, corsHeaders, corsCredentials)(mux)))
}
