package handlers

import (
	"context"
	"errors"
	"net/http"

	"example.com/mishis4x/pricesync"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// RefreshCardPrice re-checks just cardID's shared price-source url on
// demand (the "check now" refresh button next to a card's market price -
// see fe/src/components/SetDetail.tsx), reusing the exact same
// pricesync.SyncURL the background sync loop calls for every configured
// url on its own 12h schedule. A successful refresh updates every card
// sharing that url, not just cardID - the caller (SetDetail) re-fetches
// the whole set's cards afterward rather than expecting a per-card
// response body here, which is why this returns 204 on success instead of
// a JSON payload.
//
// Not gated by ownerOnlyMiddleware/CollectionOwnerUserID - same reasoning
// as the rest of the collection-tracker routes (see ListCardsForSet's doc
// comment): TCG Republic data isn't eBay-sourced, so that gate never
// actually applied here.
func (d *Data) RefreshCardPrice(w http.ResponseWriter, r *http.Request) {
	cardID := mux.Vars(r)["cardID"]

	ctx, cancel := context.WithTimeout(r.Context(), refreshPriceTimeout)
	defer cancel()

	_, url, found, err := d.P.GetPriceSourceForCard(ctx, cardID)
	if err != nil {
		log.Error().Err(err).Str("cardID", cardID).Msg("error looking up price source for card")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "This card has no price source configured.")
		return
	}

	stats, err := pricesync.SyncURL(ctx, &d.P, url)
	if err != nil {
		if errors.Is(err, pricesync.ErrRateLimited) {
			writeJSONError(w, http.StatusTooManyRequests, "Too many refreshes right now. Please try again shortly.")
			return
		}
		log.Error().Err(err).Str("cardID", cardID).Str("url", url).Msg("error refreshing card price")
		writeJSONError(w, http.StatusBadGateway, "Could not refresh this price. Please try again.")
		return
	}

	log.Info().
		Str("cardID", cardID).
		Str("url", url).
		Int("matched", stats.Matched).
		Int("unmatched", stats.Unmatched).
		Msg("on-demand price refresh")

	w.WriteHeader(http.StatusNoContent)
}
