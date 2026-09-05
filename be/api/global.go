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
}
