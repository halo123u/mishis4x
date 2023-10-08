package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

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

	stmt, dberr := d.DB.Query(q, u.Username, u.Status, string(hashedPassword))

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
	errDb := d.DB.QueryRow(q, b.Username).Scan(&hashedPassword, &status, &id)

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
