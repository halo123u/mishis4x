package handlers

import (
	"encoding/json"
	"net/http"

	"example.com/mishis4x/api"
	"github.com/rs/zerolog/log"
)

func (d *Data) CreateLobby(w http.ResponseWriter, r *http.Request) {
	i := &api.NewGameInput{}

	if !decodeJSONBody(w, r, i) {
		return
	}

	if err := d.Lobby.AddGame(i); err != nil {
		log.Error().Err(err).Msg("error adding game")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	resp, err := json.Marshal(d.Lobby.ListGames())
	if err != nil {
		log.Error().Err(err).Msg("error marshaling games")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	// Content-Type must be set before WriteHeader - headers are locked in
	// once the status is written.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if _, err := w.Write(resp); err != nil {
		log.Error().Err(err).Msg("error writing response")
	}

	log.Debug().RawJSON("games", resp).Msg("lobby created")
}

func (d *Data) ListLobbies(w http.ResponseWriter, r *http.Request) {
	resp, err := json.Marshal(d.Lobby.Games)
	if err != nil {
		log.Error().Err(err).Msg("error marshaling games")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(resp); err != nil {
		log.Error().Err(err).Msg("error writing response")
	}

	log.Debug().RawJSON("games", resp).Msg("lobbies listed")
}
