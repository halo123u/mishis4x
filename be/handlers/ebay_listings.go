package handlers

import (
	"context"
	"net/http"

	"example.com/mishis4x/api"
	"example.com/mishis4x/ebay"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// GetEbayListings returns cardID's current eBay listings, live-fetched
// and cached for up to 6h per ebay.Service's doc comment - "check prices"
// next to a card's eBay quick-search link (see
// fe/src/components/SetDetail.tsx). Not gated by ownerOnlyMiddleware/
// CollectionOwnerUserID for the same reason RefreshCardPrice isn't: this
// only ever reveals a card's own public listing data (title, price,
// seller, link) back to the same authenticated user who asked for it,
// same "Public Display" allowance covering the rest of this app's eBay
// links already.
func (d *Data) GetEbayListings(w http.ResponseWriter, r *http.Request) {
	cardID := mux.Vars(r)["cardID"]

	ctx, cancel := context.WithTimeout(r.Context(), refreshPriceTimeout)
	defer cancel()

	code, setName, found, err := d.P.GetCardSearchInfo(ctx, cardID)
	if err != nil {
		log.Error().Err(err).Str("cardID", cardID).Msg("error looking up card for ebay listings")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "Card not found.")
		return
	}

	if d.Ebay == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "eBay listings are not configured.")
		return
	}

	query := ebay.SearchQuery(setName, code)
	listings, err := d.Ebay.GetListings(ctx, cardID, query)
	if err != nil {
		log.Error().Err(err).Str("cardID", cardID).Str("query", query).Msg("error fetching ebay listings")
		writeJSONError(w, http.StatusBadGateway, "Could not fetch eBay listings. Please try again.")
		return
	}

	respListings := make([]api.EbayListing, 0, len(listings))
	for _, l := range listings {
		respListings = append(respListings, api.EbayListing{
			ItemID:                   l.ItemID,
			Title:                    l.Title,
			PriceCents:               l.PriceCents,
			Condition:                l.Condition,
			SellerUsername:           l.SellerUsername,
			SellerFeedbackPercentage: l.SellerFeedbackPercentage,
			ItemWebURL:               l.ItemWebURL,
			ImageURL:                 l.ImageURL,
		})
	}

	writeJSON(w, http.StatusOK, api.EbayListingsResponse{Query: query, Listings: respListings})
}
