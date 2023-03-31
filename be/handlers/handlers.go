package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"example.com/mishis4x/data"
	"example.com/mishis4x/lobbymodule"
)

type Handler struct {
	D *data.Data
}

func (h Handler) UserCreate(w http.ResponseWriter, r *http.Request) {
	var u data.UserInput

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&u)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	w.WriteHeader(http.StatusCreated)

	w.Header().Set("Content-Type", "application/json")

	resp, err := h.D.CreateUser(u)

	jsonData, jsonErr := json.Marshal(resp)

	if jsonErr != nil {
		http.Error(w, jsonErr.Error(), http.StatusBadRequest)
	}

	w.Write(jsonData)

	fmt.Printf("New user: %+v", u)
}

func (h *Handler) UserLogin(w http.ResponseWriter, r *http.Request) {
	var i data.UserInput

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&i)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, isAuthenticated, err := h.D.LoginUser(i)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if isAuthenticated {
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")

		jsonData, jsonErr := json.Marshal(resp)

		if jsonErr != nil {
			http.Error(w, jsonErr.Error(), http.StatusBadRequest)
		}

		w.Write(jsonData)

		fmt.Println("USER authenticated")

	} else {
		// handle invalid password
		fmt.Println("USER is unauthorized")
		http.Error(w, err.Error(), http.StatusUnauthorized)
	}
}

func (h *Handler) CreateLobby(w http.ResponseWriter, r *http.Request) {
	i := &lobbymodule.NewGameInput{}

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&i)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	h.D.L.AddGame(i)

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")

	resp, err := json.Marshal(h.D.L.ListGames())

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	w.Write(resp)

	fmt.Println("JSON output:", string(resp))
}

func (h *Handler) ListLobbies(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	resp, err := json.Marshal(h.D.L.Games)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	w.Write(resp)

	fmt.Println("JSON output:", string(resp))
}
