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
		log.Error().Err(err).Msg("error reading session")
		writeJSONError(w, http.StatusBadRequest, "Invalid session.")
		return
	}

	userID, ok := session.Values["userID"].(int)
	if !ok {
		log.Error().Msg("session missing a valid userID")
		writeJSONError(w, http.StatusUnauthorized, "You must be logged in.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	user, err := d.P.GetUserByID(ctx, userID)
	if err != nil {
		log.Error().Err(err).Int("userID", userID).Msg("error getting user")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	resp := api.GlobalData{
		User: api.User{
			ID:       user.ID,
			Username: user.Username,
			Status:   user.Status,
		},
	}

	jsonData, err := json.Marshal(resp)
	if err != nil {
		log.Error().Err(err).Msg("error marshaling global data")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	// Content-Type must be set before WriteHeader - headers are locked in
	// once the status is written.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(jsonData); err != nil {
		log.Error().Err(err).Msg("error writing response")
	}
}
