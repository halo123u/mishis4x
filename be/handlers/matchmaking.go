package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"example.com/mishis4x/api"
)

func (d *Data) CreateLobby(w http.ResponseWriter, r *http.Request) {
	i := &api.NewGameInput{}

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&i)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	d.Lobby.AddGame(i)

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")

	resp, err := json.Marshal(d.Lobby.ListGames())

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	w.Write(resp)

	fmt.Println("JSON output:", string(resp))
}

func (d *Data) ListLobbies(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	resp, err := json.Marshal(d.Lobby.Games)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	w.Write(resp)

	fmt.Println("JSON output:", string(resp))
}
