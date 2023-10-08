package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"example.com/mishis4x/handlers"
	"example.com/mishis4x/matchmaking"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type Data struct {
	db *sql.DB
	l  *matchmaking.Lobby
}

func main() {
	envPath := "./infra/envs/local/.env"
	err := godotenv.Load(envPath)

	if err != nil {
		log.Fatalf("error loading .env file: %v", err)
	}

	db, err := sql.Open("mysql", os.Getenv("DB_URL"))

	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(10)
	port := 8091

	h := handlers.Data{
		DB: db,
		Lobby: &matchmaking.Lobby{
			Games:  []*matchmaking.Game{},
			GameID: 1,
		},
	}

	h.InitializeHttpServer(port)

	fmt.Printf("Running server on port: %d\n", port)
}
