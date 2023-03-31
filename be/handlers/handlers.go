package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"example.com/mishis4x/data"
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
