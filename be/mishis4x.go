package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	_ "github.com/go-sql-driver/mysql"
)

type DB struct {
	db *sql.DB
}

type NewUser struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Status   string `json:"status"`
}

func (h *DB) UserCreate(w http.ResponseWriter, r *http.Request) {
	var nu NewUser
	nu.Status = "active"

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&nu)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	hashedPassword, cErr := bcrypt.GenerateFromPassword([]byte(nu.Password), bcrypt.DefaultCost)

	if cErr != nil {
		http.Error(w, cErr.Error(), http.StatusBadRequest)
	}

	q := `
		INSERT INTO user (username, status, password)
		VALUES (?, ?, ?);
		`
	stmt, dberr := h.db.Query(q, nu.Username, nu.Status, hashedPassword)

	if dberr != nil {
		http.Error(w, dberr.Error(), http.StatusBadGateway)
	}

	defer stmt.Close()

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
