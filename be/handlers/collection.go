package handlers

import (
	"bytes"
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

// DeleteOwnedSet removes the set named by the {setID} path variable from
// the authenticated user's collection - both the owned_sets row and every
// owned_cards row for one of its cards (see persist.DeleteOwnedSet), so
// re-adding the set later starts clean rather than resurrecting old
// ownership data. Idempotent like AddOwnedSet's counterpart: deleting a set
// that was never onboarded is still a 204, not an error.
func (d *Data) DeleteOwnedSet(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r)
	if !ok {
		log.Error().Msg("DeleteOwnedSet called without an authenticated user in context")
		writeJSONError(w, http.StatusUnauthorized, "You must be logged in.")
		return
	}

	setID := mux.Vars(r)["setID"]

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	if err := d.P.DeleteOwnedSet(ctx, userID, setID); err != nil {
		log.Error().Err(err).Int("userID", userID).Str("setID", setID).Msg("error deleting owned set")
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
		resp = append(resp, api.OwnedCardInput{
			CardID:         oc.CardID,
			Quantity:       oc.Quantity,
			PricePaidCents: oc.PricePaidCents,
		})
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
		cards = append(cards, persist.CardQuantity{
			CardID:         c.CardID,
			Quantity:       c.Quantity,
			PricePaidCents: c.PricePaidCents,
		})
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

	// Not fatal if this fails - market price is a nice-to-have overlay on
	// top of the catalog, not something the page can't function without;
	// worth logging but not worth failing the whole card list over.
	marketPrices, err := d.P.GetLatestMarketPricesForSet(ctx, setID)
	if err != nil {
		log.Error().Err(err).Str("setID", setID).Msg("error getting market prices")
		marketPrices = nil
	}

	resp := make([]api.Card, 0, len(cards))
	for _, c := range cards {
		card := api.Card{
			ID:     c.ID,
			SetID:  c.SetID,
			Name:   c.Name,
			Code:   c.Code,
			Rarity: c.Rarity,
		}
		if mp, ok := marketPrices[c.ID]; ok {
			card.MarketPriceCents = mp.PriceCents
			card.MarketCheckedAt = mp.CheckedAt
		}
		resp = append(resp, card)
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetCardImage streams the stored reference image for the card named by
// the {cardID} path variable. 404s (as a plain "Image not found.", not
// JSON - this endpoint's success response isn't JSON either) if cardID has
// no image yet, which is an ordinary, expected state rather than an error -
// image coverage is populated incrementally via process-set --images-dir,
// not guaranteed for every card.
func (d *Data) GetCardImage(w http.ResponseWriter, r *http.Request) {
	cardID := mux.Vars(r)["cardID"]

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	image, contentType, updatedAt, err := d.P.GetCardImage(ctx, cardID)
	if err != nil {
		if errors.Is(err, persist.ErrCardImageNotFound) {
			http.Error(w, "Image not found.", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("cardID", cardID).Msg("error getting card image")
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}

	// Reference art doesn't change once imported in the common case, so a
	// full day of blind browser caching (max-age) is safe - but it does
	// occasionally get replaced wholesale (e.g. a re-scrape swapping in a
	// higher-res version), so this also sets Last-Modified from the
	// image's real updated_at. ServeContent uses that for conditional
	// requests: once max-age expires, a revalidation returns a cheap 304
	// (no image bytes at all) if nothing changed, and picks up a genuine
	// replacement correctly rather than serving stale bytes for the rest
	// of the cache window. Content-Type is set explicitly first since
	// ServeContent would otherwise try to guess it from a filename we
	// don't have.
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, "", updatedAt, bytes.NewReader(image))
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
