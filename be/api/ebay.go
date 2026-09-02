package api

// EbayListing is one individual eBay seller listing for a card, as
// returned by GET /api/cards/{cardID}/ebay-listings - the wire-format
// twin of ebay.Listing (see its doc comment for field meanings), kept as
// its own type here rather than reusing ebay.Listing directly so this
// package stays the one place every JSON response shape is defined (and
// generate-types only ever needs to look at be/api/*.go).
type EbayListing struct {
	ItemID                   string `json:"item_id"`
	Title                    string `json:"title"`
	PriceCents               int    `json:"price_cents"`
	Condition                string `json:"condition"`
	SellerUsername           string `json:"seller_username"`
	SellerFeedbackPercentage string `json:"seller_feedback_percentage"`
	ItemWebURL               string `json:"item_web_url"`
	ImageURL                 string `json:"image_url"`
}

// EbayListingsResponse is what GET /api/cards/{cardID}/ebay-listings
// actually returns - Query is the same search string ebay.SearchQuery
// built server-side to fetch these Listings, handed back so the frontend
// can link out to a plain eBay search (fe/src/ebay.ts's
// ebaySearchUrlForQuery) without needing its own separate lookup of the
// card's set name just to reconstruct it.
type EbayListingsResponse struct {
	Query    string        `json:"query"`
	Listings []EbayListing `json:"listings"`
}
