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
