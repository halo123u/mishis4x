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

// ListOwnedSets returns the sets the authenticated user has onboarded -
// what the collection dashboard actually shows, as opposed to ListSets'
// full catalog. Starts empty for a fresh user even once the catalog isn't.
func (d *Data) ListOwnedSets(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r)
	if !ok {
		// AuthMiddleware already gates this route - see GetGlobalData's
		// identical comment for why this would mean a programming error.
		log.Error().Msg("ListOwnedSets called without an authenticated user in context")
		writeJSONError(w, http.StatusUnauthorized, "You must be logged in.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	sets, err := d.P.ListOwnedSets(ctx, userID)
	if err != nil {
		log.Error().Err(err).Int("userID", userID).Msg("error listing owned sets")
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

// AddOwnedSet onboards the set named in the request body for the
// authenticated user - the "add a set" step of the onboarding flow.
// Idempotent (see persist.SetOwnedSet); 404s on an unknown set_id rather
// than silently onboarding a garbage ID.
func (d *Data) AddOwnedSet(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r)
	if !ok {
		log.Error().Msg("AddOwnedSet called without an authenticated user in context")
		writeJSONError(w, http.StatusUnauthorized, "You must be logged in.")
		return
	}

	var input api.AddOwnedSetInput
	if !decodeJSONBody(w, r, &input) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	if _, err := d.P.GetSet(ctx, input.SetID); err != nil {
		if errors.Is(err, persist.ErrSetNotFound) {
			writeJSONError(w, http.StatusNotFound, "Set not found.")
			return
		}
		log.Error().Err(err).Str("setID", input.SetID).Msg("error getting set")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	if err := d.P.SetOwnedSet(ctx, userID, input.SetID); err != nil {
		log.Error().Err(err).Int("userID", userID).Str("setID", input.SetID).Msg("error onboarding set")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListOwnedCardsForSet returns the authenticated user's ownership rows for
// the set named by the {setID} path variable - what SetDetail uses to tell
// owned cards apart from missing ones, and what the set-editor form uses to
// pre-fill its checkboxes/quantities. A card the user has never interacted
// with simply doesn't appear in the response, same as ListOwnedCardsBySet.
func (d *Data) ListOwnedCardsForSet(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r)
	if !ok {
		log.Error().Msg("ListOwnedCardsForSet called without an authenticated user in context")
		writeJSONError(w, http.StatusUnauthorized, "You must be logged in.")
		return
	}

	setID := mux.Vars(r)["setID"]

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	owned, err := d.P.ListOwnedCardsBySet(ctx, userID, setID)
	if err != nil {
		log.Error().Err(err).Int("userID", userID).Str("setID", setID).Msg("error listing owned cards")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	resp := make([]api.OwnedCardInput, 0, len(owned))
	for _, oc := range owned {
		resp = append(resp, api.OwnedCardInput{CardID: oc.CardID, Quantity: oc.Quantity})
	}

	writeJSON(w, http.StatusOK, resp)
}

// SetOwnedCardsForSet records which cards of the set named by the {setID}
// path variable the authenticated user owns, and in what quantity - the
// card-selection step of onboarding, submitted after AddOwnedSet has
// already onboarded the set itself. Only accepts card IDs that actually
// belong to setID, rather than trusting the client to only ever submit
// ones that do.
func (d *Data) SetOwnedCardsForSet(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r)
	if !ok {
		log.Error().Msg("SetOwnedCardsForSet called without an authenticated user in context")
		writeJSONError(w, http.StatusUnauthorized, "You must be logged in.")
		return
	}

	setID := mux.Vars(r)["setID"]

	var input api.SetOwnedCardsInput
	if !decodeJSONBody(w, r, &input) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	catalogCards, err := d.P.ListCardsBySet(ctx, setID)
	if err != nil {
		log.Error().Err(err).Str("setID", setID).Msg("error listing cards for set")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	validCardIDs := make(map[string]bool, len(catalogCards))
	for _, c := range catalogCards {
		validCardIDs[c.ID] = true
	}

	cards := make([]persist.CardQuantity, 0, len(input.Cards))
	for _, c := range input.Cards {
		if !validCardIDs[c.CardID] {
			writeJSONError(w, http.StatusBadRequest, "One of these cards doesn't belong to this set.")
			return
		}
		cards = append(cards, persist.CardQuantity{CardID: c.CardID, Quantity: c.Quantity})
	}

	if err := d.P.SetOwnedCards(ctx, userID, cards); err != nil {
		log.Error().Err(err).Int("userID", userID).Str("setID", setID).Msg("error setting owned cards")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
