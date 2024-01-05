package handlers

import (
	"encoding/json"
	"net/http"

	"example.com/mishis4x/api"
)

func (d *Data) GetGlobalData(w http.ResponseWriter, r *http.Request) {
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

	resp := api.GlobalData{
		User: api.User{
		ID:       user.ID,
		Username: user.Username,
		Status:   user.Status,
		},
	}

	jsonData, jsonErr := json.Marshal(resp)

	if jsonErr != nil {
		http.Error(w, jsonErr.Error(), http.StatusBadRequest)
	}

	w.Write(jsonData)
}
