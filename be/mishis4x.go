package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"example.com/mishis4x/lobbymodule"
	"golang.org/x/crypto/bcrypt"

	_ "github.com/go-sql-driver/mysql"
)

type Data struct {
	db *sql.DB
	l  *lobbymodule.Lobby
}

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Status   string `json:"status"`
}

func (d *Data) UserCreate(w http.ResponseWriter, r *http.Request) {
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

	stmt, dberr := d.db.Query(q, u.Username, u.Status, string(hashedPassword))

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

func (d *Data) UserLogin(w http.ResponseWriter, r *http.Request) {
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
	errDb := d.db.QueryRow(q, b.Username).Scan(&hashedPassword, &status, &id)

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
				"user_id":  id,
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

func (d *Data) CreateLobby(w http.ResponseWriter, r *http.Request) {
	i := &lobbymodule.NewGameInput{}

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&i)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	d.l.AddGame(i)

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")

	resp, err := json.Marshal(d.l.ListGames())

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	w.Write(resp)

	fmt.Println("JSON output:", string(resp))

}

func (d *Data) ListLobbies(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	resp, err := json.Marshal(d.l.Games)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	w.Write(resp)

	fmt.Println("JSON output:", string(resp))
}

func main() {
	db, err := sql.Open("mysql", "user1:password@/mishis4x")

	if err != nil {
		panic(err)
	}

	d := Data{
		db: db,
		l: &lobbymodule.Lobby{
			Games:  []*lobbymodule.Game{},
			GameID: 1,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/user/create", d.UserCreate)
	mux.HandleFunc("/user/login", d.UserLogin)
	mux.HandleFunc("/lobbies", d.ListLobbies)
	mux.HandleFunc("/lobbies/create", d.CreateLobby)

	db.SetMaxOpenConns(10)

	port := 8091

	fmt.Printf("Running server on port: %d\n", port)

	log.Fatal(http.ListenAndServe(":8091", mux))
}
