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

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Status   string `json:"status"`
}

func (h *DB) UserCreate(w http.ResponseWriter, r *http.Request) {
	var u User
	u.Status = "active"

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&u)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	hashedPassword, cErr := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)

	if cErr != nil {
		http.Error(w, cErr.Error(), http.StatusBadRequest)
	}

	q := `
		INSERT INTO user (username, status, password)
		VALUES (?, ?, ?);
		`

	stmt, dberr := h.db.Query(q, u.Username, u.Status, string(hashedPassword))

	if dberr != nil {
		http.Error(w, dberr.Error(), http.StatusBadRequest)
	} else {
		defer stmt.Close()
		w.WriteHeader(http.StatusCreated)

		w.Header().Set("Content-Type", "application/json")

		resp := map[string]interface{}{
			"username": u.Username,
			"status":   u.Status,
		}

		jsonData, jsonErr := json.Marshal(resp)

		if jsonErr != nil {
			http.Error(w, jsonErr.Error(), http.StatusBadRequest)
		}

		w.Write(jsonData)

		fmt.Printf("New user: %+v", u)
	}
}

type LoginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *DB) UserLogin(w http.ResponseWriter, r *http.Request) {
	var b LoginBody

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&b)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	q := `
		SELECT password, status, id FROM user
		WHERE username = (?);
	`

	var hashedPassword string
	var status string
	var id string
	errDb := h.db.QueryRow(q, b.Username).Scan(&hashedPassword, &status, &id)

	if errDb != nil {
		http.Error(w, errDb.Error(), http.StatusBadRequest)
	} else {

		err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(b.Password))

		if err != nil {
			// handle invalid password
			fmt.Println("USER is unauthorized")
			http.Error(w, err.Error(), http.StatusUnauthorized)
		} else {

			w.WriteHeader(http.StatusCreated)
			w.Header().Set("Content-Type", "application/json")

			resp := map[string]interface{}{
				"username": b.Username,
				"status":   status,
			}
			jsonData, jsonErr := json.Marshal(resp)

			if jsonErr != nil {
				http.Error(w, jsonErr.Error(), http.StatusBadRequest)
			}

			w.Write(jsonData)

			fmt.Println("USER authenticated")
		}
	}
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
	mux.HandleFunc("/user/login", d.UserLogin)

	db.SetMaxOpenConns(10)

	port := 8091

	fmt.Printf("Running server on port: %d\n", port)

	log.Fatal(http.ListenAndServe(":8091", mux))
}
