package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
)

type DB struct {
	db *sql.DB
}

type NewUser struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *DB) UserCreate(w http.ResponseWriter, r *http.Request) {
	var nu NewUser

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&nu)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	fmt.Printf("New user: %+v", nu)

}

func main() {
	db, err := sql.Open("mysql", "user1:password@/mishis4x")

	if err != nil {
		panic(err)
	}

	d := DB{
		db: db,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/user/create", d.UserCreate)

	db.SetMaxOpenConns(10)

	port := 8091

	fmt.Printf("Running server on port: %d", port)

	log.Fatal(http.ListenAndServe(":8091", mux))
}
