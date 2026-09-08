package api

type GlobalData struct {
	User User `json:"user"`
	// EbayListingsEnabled gates whether the frontend shows the "eBay"
	// price-source option at all - not just whether credentials happen to
	// be configured (see handlers.Data.EbayListingsDisabled's doc comment
	// for why this is a separate kill switch).
	EbayListingsEnabled bool `json:"ebay_listings_enabled"`
	// PriceTrendsEnabled gates whether the frontend shows the per-card
	// price-trend icon at all - see handlers.Data.PriceTrendsEnabled's
	// doc comment for why this ships off by default rather than on.
	PriceTrendsEnabled bool `json:"price_trends_enabled"`
	// ModelViewerEnabled gates whether the frontend shows any link to the
	// character model viewer - true only for the one account
	// handlers.Data.ModelViewerUserID names, same single-owner shape as
	// IsAdmin above but a deliberately separate check (copyright
	// reasoning, not app administration - see ModelViewerUserID's own doc
	// comment).
	ModelViewerEnabled bool `json:"model_viewer_enabled"`
}
