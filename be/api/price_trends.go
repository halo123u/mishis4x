package api

type DailyPricePoint struct {
	Date       string `json:"date"`
	PriceCents int    `json:"price_cents"`
}

// CardPriceTrend mirrors persist.CardPriceTrend directly - unlike
// api.AdminInviteRequest, there's nothing sensitive to withhold here
// (catalog-level price history, same visibility as the market price
// already shown on every card tile), so no separate shaping is needed.
type CardPriceTrend struct {
	CardID        string            `json:"card_id"`
	DailyPrices   []DailyPricePoint `json:"daily_prices"`
	ChangeCents   int               `json:"change_cents"`
	ChangePercent float64           `json:"change_percent"`
}

// CardPriceMover mirrors persist.CardPriceMover directly - same "nothing
// sensitive to withhold" reasoning as CardPriceTrend above, plus Name/
// SetID so the Home page's movers widget can render/link a result
// without a second round trip per card.
type CardPriceMover struct {
	CardID        string  `json:"card_id"`
	SetID         string  `json:"set_id"`
	Name          string  `json:"name"`
	PreviousDate  string  `json:"previous_date"`
	PreviousCents int     `json:"previous_cents"`
	LatestDate    string  `json:"latest_date"`
	LatestCents   int     `json:"latest_cents"`
	ChangeCents   int     `json:"change_cents"`
	ChangePercent float64 `json:"change_percent"`
}
