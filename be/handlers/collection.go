package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"example.com/mishis4x/api"
	"example.com/mishis4x/persist"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// ListSets returns every set in the catalog.
func (d *Data) ListSets(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	sets, err := d.P.ListSets(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error listing sets")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	resp := make([]api.Set, 0, len(sets))
	for _, s := range sets {
		resp = append(resp, api.Set{
			ID:          s.ID,
			Name:        s.Name,
			CardCount:   s.CardCount,
			ReleaseDate: s.ReleaseDate,
			Status:      s.Status,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// ListCardsForSet returns every card belonging to the set named by the
// {setID} path variable. 404s if setID doesn't match a real set, rather
// than silently returning an empty list indistinguishable from "this set
// exists and just has no cards yet".
func (d *Data) ListCardsForSet(w http.ResponseWriter, r *http.Request) {
	setID := mux.Vars(r)["setID"]

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	if _, err := d.P.GetSet(ctx, setID); err != nil {
		if errors.Is(err, persist.ErrSetNotFound) {
			writeJSONError(w, http.StatusNotFound, "Set not found.")
			return
		}
		log.Error().Err(err).Str("setID", setID).Msg("error getting set")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	cards, err := d.P.ListCardsBySet(ctx, setID)
	if err != nil {
		log.Error().Err(err).Str("setID", setID).Msg("error listing cards")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	resp := make([]api.Card, 0, len(cards))
	for _, c := range cards {
		resp = append(resp, api.Card{
			ID:     c.ID,
			SetID:  c.SetID,
			Name:   c.Name,
			Code:   c.Code,
			Rarity: c.Rarity,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// writeJSON marshals v and writes it as the response body with status.
// Existing handlers (ListLobbies, CreateLobby, GetGlobalData) each repeat
// this marshal/header/write sequence inline - factored out here rather than
// copied a third time, but deliberately not retrofitted onto those to keep
// this change scoped to the new collection endpoints.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		log.Error().Err(err).Msg("error marshaling response")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		log.Error().Err(err).Msg("error writing response")
	}
}
