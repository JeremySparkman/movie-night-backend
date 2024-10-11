package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type Vote struct {
	Voter string `json:"voter"`
	Score string `json:"score"`
}

var (
	voteCounts = make(map[string]int)
	voters     = make(map[string]bool)
	mutex      sync.Mutex
	clients    = make(map[*websocket.Conn]bool)
	broadcast  = make(chan map[string]interface{})
	upgrader   = websocket.Upgrader{}
)

func voteHandler(w http.ResponseWriter, r *http.Request) {
	var vote Vote
	err := json.NewDecoder(r.Body).Decode(&vote)
	if err != nil || vote.Voter == "" || vote.Score == "" {
		http.Error(w, "Invalid vote data", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	if voters[vote.Voter] {
		http.Error(w, "Voter has already voted", http.StatusForbidden)
		return
	}

	voteCounts[vote.Score]++
	voters[vote.Voter] = true

	update := map[string]interface{}{
		"voter":  vote.Voter,
		"score":  vote.Score,
		"counts": voteCounts,
	}

	broadcast <- update

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
		_, _, err := ws.ReadMessage()
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
			err := client.WriteJSON(update)
			fmt.Printf(client.LocalAddr().String())
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
	http.HandleFunc("/vote", voteHandler)
	http.HandleFunc("/ws", handleConnections)

	go handleMessages()

	fmt.Println("Server starting...")
	log.Fatal(http.ListenAndServe(":443", nil))
}
