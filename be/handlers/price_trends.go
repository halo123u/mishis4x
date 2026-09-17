package handlers

import (
	"context"
	"errors"
	"net/http"

	"example.com/mishis4x/api"
	"example.com/mishis4x/persist"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// GetPriceTrends is the per-card price-trend feature's one endpoint -
// gated by PriceTrendsEnabled both here (defense in depth, same
// convention as GetEbayListings/EbayListingsDisabled) and in
// GetGlobalData (so the frontend never shows the trend icon at all while
// this is off). See persist.GetPriceTrendsForSet for what's actually
// returned - only cards with at least 2 days of TCG Republic price
// history in the last week show up at all.
func (d *Data) GetPriceTrends(w http.ResponseWriter, r *http.Request) {
	if !d.PriceTrendsEnabled {
		writeJSONError(w, http.StatusServiceUnavailable, "Price trends are not available right now.")
		return
	}

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

	trends, err := d.P.GetPriceTrendsForSet(ctx, setID)
	if err != nil {
		log.Error().Err(err).Str("setID", setID).Msg("error getting price trends")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	resp := make([]api.CardPriceTrend, 0, len(trends))
	for _, t := range trends {
		points := make([]api.DailyPricePoint, len(t.DailyPrices))
		for i, p := range t.DailyPrices {
			points[i] = api.DailyPricePoint{Date: p.Date, PriceCents: p.PriceCents}
		}
		resp = append(resp, api.CardPriceTrend{
			CardID:        t.CardID,
			DailyPrices:   points,
			ChangeCents:   t.ChangeCents,
			ChangePercent: t.ChangePercent,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetOwnedCardPriceMovers is the "today's biggest movers" Home-page
// widget's one endpoint - same PriceTrendsEnabled gate as GetPriceTrends
// (this is the same underlying TCG Republic price-history feature, just
// a day-over-day view scoped to the caller's own owned cards instead of
// a whole set's 7-day trend), and the same defense-in-depth reasoning
// for checking it here too, not just in GetGlobalData. Open to any
// authenticated user for their own cards - not owner-gated, same as
// every other collection-tracker route (see the /sets subrouter's own
// doc comment in handlers.go for why: nothing here is eBay-sourced).
func (d *Data) GetOwnedCardPriceMovers(w http.ResponseWriter, r *http.Request) {
	if !d.PriceTrendsEnabled {
		writeJSONError(w, http.StatusServiceUnavailable, "Price trends are not available right now.")
		return
	}

	userID, ok := userIDFromContext(r)
	if !ok {
		// AuthMiddleware already gates this route - see GetGlobalData's
		// identical comment for why this would mean a programming error.
		log.Error().Msg("GetOwnedCardPriceMovers called without an authenticated user in context")
		writeJSONError(w, http.StatusUnauthorized, "You must be logged in.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	movers, err := d.P.GetOwnedCardPriceMovers(ctx, userID)
	if err != nil {
		log.Error().Err(err).Int("userID", userID).Msg("error getting owned card price movers")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	resp := make([]api.CardPriceMover, 0, len(movers))
	for _, m := range movers {
		resp = append(resp, api.CardPriceMover{
			CardID:        m.CardID,
			SetID:         m.SetID,
			Name:          m.Name,
			PreviousDate:  m.PreviousDate,
			PreviousCents: m.PreviousCents,
			LatestDate:    m.LatestDate,
			LatestCents:   m.LatestCents,
			ChangeCents:   m.ChangeCents,
			ChangePercent: m.ChangePercent,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}
