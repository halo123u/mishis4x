package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"example.com/mishis4x/data"
	"example.com/mishis4x/handlers"
	"example.com/mishis4x/lobbymodule"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	db, err := sql.Open("mysql", "user1:password@/mishis4x")

	if err != nil {
		panic(err)
	}

	// data layer
	d := &data.Data{
		DB: db,
		L: &lobbymodule.Lobby{
			Games:  []*lobbymodule.Game{},
			GameID: 1,
		},
	}
	// handlers layer
	h := handlers.Handler{
		D: d,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/user/create", h.UserCreate)
	mux.HandleFunc("/user/login", h.UserLogin)
	mux.HandleFunc("/lobbies", h.ListLobbies)
	mux.HandleFunc("/lobbies/create", h.CreateLobby)

	db.SetMaxOpenConns(10)

	port := 8091

	fmt.Printf("Running server on port: %d\n", port)

	log.Fatal(http.ListenAndServe(":8091", mux))
}
