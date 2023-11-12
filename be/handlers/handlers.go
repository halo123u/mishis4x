package handlers

import (
	"fmt"
	"log"
	"net/http"

	"example.com/mishis4x/matchmaking"
	"example.com/mishis4x/persist"
)

type Data struct {
	P     persist.Persist
	Lobby *matchmaking.Lobby
}

func (d *Data) InitializeHttpServer(port int) {

	mux := http.NewServeMux()
	mux.HandleFunc("/user/create", d.UserCreate)
	mux.HandleFunc("/user/login", d.UserLogin)
	mux.HandleFunc("/lobbies", d.ListLobbies)
	mux.HandleFunc("/lobbies/create", d.CreateLobby)
	log.Printf("Running server on port: %d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), mux))
}
