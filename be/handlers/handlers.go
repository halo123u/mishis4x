package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"example.com/mishis4x/matchmaking"
)

type Data struct {
	DB    *sql.DB
	Lobby *matchmaking.Lobby
}

func (d *Data) InitializeHttpServer(port int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/user/create", d.UserCreate)
	mux.HandleFunc("/user/login", d.UserLogin)
	mux.HandleFunc("/lobbies", d.ListLobbies)
	mux.HandleFunc("/lobbies/create", d.CreateLobby)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), mux))
}