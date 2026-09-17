package persist

import (
	"context"
	"sort"

	sq "github.com/Masterminds/squirrel"
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

// CardPriceMover is one owned card's most recent day-over-day TCG
// Republic price move - "daily" in the sense of "since the last day we
// actually have data for", not a guaranteed exact 24h comparison: if the
// sync job gaps for a few days, this still compares the two most recent
// real data points rather than requiring them to be calendar-adjacent.
// Name/SetID ride along so a caller (the Home page's movers widget) can
// render/link a result without a second round trip per card.
type CardPriceMover struct {
	CardID        string
	SetID         string
	Name          string
	PreviousDate  string
	PreviousCents int
	LatestDate    string
	LatestCents   int
	ChangeCents   int
	ChangePercent float64
}

// GetOwnedCardPriceMovers returns a mover for every card userID owns
// (at least one owned_card_copies row) that has at least 2 distinct days
// of TCG Republic price history within the last priceTrendWindowDays -
// same "not present means nothing to report" convention as
// GetPriceTrendsForSet, just day-over-day instead of first-vs-last
// across the whole window (this is the "daily" movers list, not the
// per-card 7-day trend chart). Sorted by |ChangeCents| descending, so
// the biggest movers - up or down - are already first; a caller wanting
// "today's top gainers/losers" can just take the front of the slice
// without re-sorting.
func (p *Persist) GetOwnedCardPriceMovers(ctx context.Context, userID int) ([]CardPriceMover, error) {
	// Three layers: innermost picks one row per (card, day) - the day's
	// last check, same as GetPriceTrendsForSet's own ranked subquery.
	// The middle layer re-ranks those day-rows per card, most recent day
	// first (day_rank). The outer WHERE keeps only day_rank 1 and 2 - a
	// card's latest day and the one before it - everything else (a card
	// with only 1 day of history, or days further back) is dropped before
	// it ever reaches Go.
	rows, err := p.DB.QueryContext(ctx, `
		SELECT card_id, day, price_cents, day_rank
		FROM (
			SELECT
				card_id, day, price_cents,
				ROW_NUMBER() OVER (
					PARTITION BY card_id ORDER BY day DESC
				) AS day_rank
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
				JOIN (
					SELECT DISTINCT card_id FROM owned_card_copies WHERE user_id = ?
				) owned ON owned.card_id = card_price_history.card_id
				WHERE card_price_history.source = 'tcg_republic'
					AND card_price_history.price_cents IS NOT NULL
					AND card_price_history.recorded_at >= DATE_SUB(NOW(), INTERVAL ? DAY)
			) one_row_per_card_day
			WHERE rn = 1
		) ranked_days
		WHERE day_rank <= 2
		ORDER BY card_id, day_rank ASC
	`, userID, priceTrendWindowDays)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing rows")
		}
	}()

	// order tracks first-seen card_id order (stable, since the query above
	// is itself ordered by card_id) - byCard alone (a plain map) wouldn't
	// preserve that.
	var order []string
	byCard := map[string][]DailyPricePoint{}
	for rows.Next() {
		var cardID, day string
		var priceCents, dayRank int
		if err := rows.Scan(&cardID, &day, &priceCents, &dayRank); err != nil {
			return nil, err
		}
		if _, ok := byCard[cardID]; !ok {
			order = append(order, cardID)
		}
		// day_rank 1 (latest) always arrives before day_rank 2 (previous)
		// per card, per the query's own ORDER BY - appending in arrival
		// order is enough to keep index 0 = latest, index 1 = previous.
		byCard[cardID] = append(byCard[cardID], DailyPricePoint{Date: day, PriceCents: priceCents})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var cardIDs []string
	for _, cardID := range order {
		if len(byCard[cardID]) == 2 {
			cardIDs = append(cardIDs, cardID)
		}
	}
	if len(cardIDs) == 0 {
		return nil, nil
	}

	names, err := p.getCardNamesAndSets(ctx, cardIDs)
	if err != nil {
		return nil, err
	}

	movers := make([]CardPriceMover, 0, len(cardIDs))
	for _, cardID := range cardIDs {
		points := byCard[cardID]
		latest, previous := points[0], points[1]
		changeCents := latest.PriceCents - previous.PriceCents
		// previous.PriceCents == 0 shouldn't happen in real data (see
		// GetPriceTrendsForSet's identical guard) - kept for the same
		// no-Inf/NaN-in-the-response reason.
		var changePercent float64
		if previous.PriceCents != 0 {
			changePercent = float64(changeCents) / float64(previous.PriceCents) * 100
		}

		info := names[cardID]
		movers = append(movers, CardPriceMover{
			CardID:        cardID,
			SetID:         info.SetID,
			Name:          info.Name,
			PreviousDate:  previous.Date,
			PreviousCents: previous.PriceCents,
			LatestDate:    latest.Date,
			LatestCents:   latest.PriceCents,
			ChangeCents:   changeCents,
			ChangePercent: changePercent,
		})
	}

	sort.Slice(movers, func(i, j int) bool {
		return abs(movers[i].ChangeCents) > abs(movers[j].ChangeCents)
	})

	return movers, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

type cardNameAndSet struct {
	Name  string
	SetID string
}

// getCardNamesAndSets looks up Name/SetID for a batch of card ids - split
// out of GetOwnedCardPriceMovers purely so that function's own query
// doesn't need a fourth JOIN layered onto an already-nested window-
// function query; cardIDs here is already small (a user's own owned-and-
// price-moved cards, not the whole catalog).
func (p *Persist) getCardNamesAndSets(ctx context.Context, cardIDs []string) (map[string]cardNameAndSet, error) {
	rows, err := sq.Select("id", "name", "set_id").
		From("cards").
		Where(sq.Eq{"id": cardIDs}).
		RunWith(p.DB).
		QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing rows")
		}
	}()

	result := make(map[string]cardNameAndSet, len(cardIDs))
	for rows.Next() {
		var id, name, setID string
		if err := rows.Scan(&id, &name, &setID); err != nil {
			return nil, err
		}
		result[id] = cardNameAndSet{Name: name, SetID: setID}
	}
	return result, rows.Err()
}
