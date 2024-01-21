package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"example.com/mishis4x/api"
	"example.com/mishis4x/persist"
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
		log.Printf("Error decoding user: %+v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)

	if err != nil {
		log.Printf("Error hashing password: %+v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	id, err := d.P.CreateUser(persist.User{
		Username: u.Username,
		Password: string(hashedPassword),
		Status:   u.Status,
	})

	if err != nil {
		log.Printf("Error creating user: %+v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		log.Printf("Error getting session: %+v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	session.Values["userID"] = id
	session.Values["authenticated"] = true
	// saves cookie
	err = session.Save(r, w)
	if err != nil {
		log.Println("Error saving session on create")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusCreated)

	log.Printf("New user: %+v", u)
}

func (d *Data) UserLogin(w http.ResponseWriter, r *http.Request) {
	var b api.UserLogin

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&b)
	if err != nil {
		log.Printf("Error decoding user: %+v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	u, err := d.P.GetUserByUsername(b.Username)
	if err != nil {
		log.Printf("Error getting user: %+v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	err = bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(b.Password))

	if err != nil {
		log.Printf("Error comparing password: %+v", err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
	}
	log.Printf("USER authenticated")
	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		log.Printf("Error getting session: %+v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	session.Values["userID"] = u.ID
	session.Values["authenticated"] = true
	err = session.Save(r, w)
	if err != nil {
		log.Printf("Error saving session on login: %+v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

// TODO: maybe move to its own file ?
func (d *Data) UserLogout(w http.ResponseWriter, r *http.Request) {
	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	session.Values["authenticated"] = false
	session.Options.MaxAge = -1
	err = session.Save(r, w)
	if err != nil {
		fmt.Println("Error saving session")
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
