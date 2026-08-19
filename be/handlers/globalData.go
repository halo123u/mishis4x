package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"example.com/mishis4x/api"
	"github.com/rs/zerolog/log"
)

func (d *Data) GetGlobalData(w http.ResponseWriter, r *http.Request) {
	session, err := d.Sessions.Get(r, "session")

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	userID := session.Values["userID"]

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	user, err := d.P.GetUserByID(ctx, userID.(int))
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

	if _, err := w.Write(jsonData); err != nil {
		log.Error().Err(err).Msg("error writing response")
	}
}
