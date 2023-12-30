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
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	id, err := d.P.CreateUser(persist.User{
		Username: u.Username,
		Password: string(hashedPassword),
		Status:   u.Status,
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	user, err := d.P.GetUserByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")

	resp := api.User{
		ID:       user.ID,
		Username: user.Username,
		Status:   user.Status,
	}

	jsonData, jsonErr := json.Marshal(resp)

	if jsonErr != nil {
		http.Error(w, jsonErr.Error(), http.StatusBadRequest)
	}

	w.Write(jsonData)

	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	session.Values["user"] = u.Username
	session.Values["authenticated"] = true
	// saves cookie
	session.Save(r, w)

	log.Printf("New user: %+v", u)
}

func (d *Data) UserLogin(w http.ResponseWriter, r *http.Request) {
	var b api.UserLogin

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	u, err := d.P.GetUserByUsername(b.Username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	err = bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(b.Password))

	if err != nil {
		// handle invalid password
		fmt.Println("USER is unauthorized")
		http.Error(w, err.Error(), http.StatusUnauthorized)
	}
	log.Printf("USER authenticated")
	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	session.Values["userID"] = u.ID
	session.Values["authenticated"] = true
	err = session.Save(r, w)
	if err != nil {
		fmt.Println("Error saving session")
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func (d *Data) GetUserData(w http.ResponseWriter, r *http.Request) {
	session, err := d.Sessions.Get(r, "session")

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	userID := session.Values["userID"]

	user, err := d.P.GetUserByID(userID.(int))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	resp := api.User{
		ID:       user.ID,
		Username: user.Username,
		Status:   user.Status,
	}

	jsonData, jsonErr := json.Marshal(resp)

	if jsonErr != nil {
		http.Error(w, jsonErr.Error(), http.StatusBadRequest)
	}

	w.Write(jsonData)
}
