package persist

import (
	"context"

	"github.com/rs/zerolog/log"
)

// priceTrendWindowDays is how far back GetPriceTrendsForSet looks -
// matches the "past week" framing the feature was actually asked for,
// not a tunable option (see ENABLE_PRICE_TRENDS's own doc comment for
// why this whole feature ships off by default rather than configurable).
const priceTrendWindowDays = 7

// DailyPricePoint is one calendar day's TCG Republic price for a card -
// only ever the last check recorded that day, not every check (the sync
// job runs twice daily; charting both would just show near-duplicate
// pairs of points most days for a source that doesn't move that often -
// see the analytics-trends-mock artifact's reasoning). A day with no
// successful price found at all (source reported nothing) simply isn't
// present - there's no zero/null point to plot, same "absence just
// means nothing to report" convention as the rest of card_price_history.
type DailyPricePoint struct {
	Date       string
	PriceCents int
}

// CardPriceTrend is one card's price movement over priceTrendWindowDays -
// only meaningful with at least 2 daily points (see
// GetPriceTrendsForSet), so ChangeCents/ChangePercent are always real
// comparisons, never a lone data point compared to itself.
type CardPriceTrend struct {
	CardID        string
	DailyPrices   []DailyPricePoint
	ChangeCents   int
	ChangePercent float64
}

// GetPriceTrendsForSet returns a trend for every card in setID that has
// at least 2 distinct days of TCG Republic price history within the last
// priceTrendWindowDays - a card with zero or exactly one day's data has
// nothing to compare, so it's simply absent from the map (same "not
// present means nothing to report" convention GetLatestMarketPricesForSet
// already uses), not included with a zeroed/misleading trend.
//
// Deliberately TCG Republic only (source = 'tcg_republic') - there's no
// eBay equivalent of this data (listings are fetched live and cached, not
// recorded into history), and mixing the two into one chart would be
// exactly the kind of "combining sourced data" eBay's API License
// Agreement is careful about (see [[ebay-api-license-terms]]), even
// though this table itself is TCG-only already.
func (p *Persist) GetPriceTrendsForSet(ctx context.Context, setID string) (map[string]CardPriceTrend, error) {
	// One row per card per calendar day (the day's last check, by
	// recorded_at) - a window function picks that in one query, same
	// "latest per group" approach GetLatestMarketPricesForSet already
	// uses, just partitioned by (card_id, day) instead of just card_id.
	// Filtering to cards.set_id = ? and a real recorded_at window happens
	// inside the same subquery so the ROW_NUMBER() partitioning only ever
	// considers rows already in scope.
	rows, err := p.DB.QueryContext(ctx, `
		SELECT card_id, day, price_cents
		FROM (
			SELECT
				card_price_history.card_id,
				DATE(card_price_history.recorded_at) AS day,
				card_price_history.price_cents,
				ROW_NUMBER() OVER (
					PARTITION BY card_price_history.card_id, DATE(card_price_history.recorded_at)
					ORDER BY card_price_history.recorded_at DESC
				) AS rn
			FROM card_price_history
			JOIN cards ON cards.id = card_price_history.card_id
			WHERE cards.set_id = ?
				AND card_price_history.source = 'tcg_republic'
				AND card_price_history.price_cents IS NOT NULL
				AND card_price_history.recorded_at >= DATE_SUB(NOW(), INTERVAL ? DAY)
		) ranked
		WHERE rn = 1
		ORDER BY card_id, day ASC
	`, setID, priceTrendWindowDays)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing rows")
		}
	}()

	byCard := make(map[string][]DailyPricePoint)
	for rows.Next() {
		var cardID, day string
		var priceCents int
		if err := rows.Scan(&cardID, &day, &priceCents); err != nil {
			return nil, err
		}
		byCard[cardID] = append(byCard[cardID], DailyPricePoint{Date: day, PriceCents: priceCents})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	trends := make(map[string]CardPriceTrend)
	for cardID, points := range byCard {
		if len(points) < 2 {
			continue
		}

		first, last := points[0], points[len(points)-1]
		changeCents := last.PriceCents - first.PriceCents
		// first.PriceCents == 0 shouldn't happen in real data (a genuinely
		// for-sale card wouldn't be recorded at $0.00 - unavailable is a
		// NULL price, already excluded above), but guarding against it
		// avoids a stray +Inf/NaN reaching the JSON response if it ever did.
		var changePercent float64
		if first.PriceCents != 0 {
			changePercent = float64(changeCents) / float64(first.PriceCents) * 100
		}

		trends[cardID] = CardPriceTrend{
			CardID:        cardID,
			DailyPrices:   points,
			ChangeCents:   changeCents,
			ChangePercent: changePercent,
		}
	}

	return trends, nil
}
