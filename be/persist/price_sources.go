package persist

import (
	"context"
	"database/sql"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rs/zerolog/log"
)

// UpsertPriceSource records where the background price-sync job (and the
// on-demand "check now" endpoint) should look for cardID's price/stock.
// source picks which parser to apply (e.g. "tcg_republic") - url itself is
// fully free-form and user-settable, not reconstructed from any
// source-specific ID template (see 20260831_12_add_card_price_sources_table.sql).
//
// Updates source/url in place on a re-run without touching
// last_checked_at - re-running set-price-sources to fix a typo'd URL
// shouldn't reset a card's staleness clock and force it to the front of
// the sync job's queue ahead of everything else's actual schedule, the
// same way UpsertCard's re-run never touches a card's id.
func (p *Persist) UpsertPriceSource(ctx context.Context, cardID, source, url string) error {
	_, err := sq.Insert("card_price_sources").
		Columns("card_id", "source", "url").
		Values(cardID, source, url).
		Suffix("ON DUPLICATE KEY UPDATE source = VALUES(source), url = VALUES(url)").
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}

// ListDistinctPriceSourceURLs returns every distinct url currently
// configured in card_price_sources. Many cards typically share the same
// url (a TCG Republic category listing page covers dozens of cards at
// once - see set-price-sources's doc comment), so the sync job fetches
// each of these once and matches every card that points at it, rather
// than fetching once per card_id.
func (p *Persist) ListDistinctPriceSourceURLs(ctx context.Context) ([]string, error) {
	rows, err := sq.Select("DISTINCT url").
		From("card_price_sources").
		OrderBy("url").
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

	var urls []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}

	return urls, rows.Err()
}

// PriceSourceCard is one card tracked against a given price-source url,
// with the code the sync job needs to find it in that page's scraped
// listing (card_price_sources itself doesn't store code - it's reached
// through cards, the same way every other price-source lookup is).
type PriceSourceCard struct {
	CardID string
	Code   string
	Source string
}

// ListPriceSourcesForURL returns every card whose card_price_sources row
// points at url, for the sync job to match against that url's scraped
// listing once it's been fetched.
func (p *Persist) ListPriceSourcesForURL(ctx context.Context, url string) ([]PriceSourceCard, error) {
	rows, err := sq.Select("card_price_sources.card_id", "cards.code", "card_price_sources.source").
		From("card_price_sources").
		Join("cards ON cards.id = card_price_sources.card_id").
		Where(sq.Eq{"card_price_sources.url": url}).
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

	var cards []PriceSourceCard
	for rows.Next() {
		var c PriceSourceCard
		if err := rows.Scan(&c.CardID, &c.Code, &c.Source); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}

	return cards, rows.Err()
}

// MarketPrice is one card's current market-price standing: PriceCents is
// nil when there's nothing to show right now (whether that's because the
// source reported no price, or because there's history data at all -
// this doesn't distinguish those; see GetLatestMarketPricesForSet's doc
// comment), and CheckedAt is nil only when the card has never actually
// been synced yet at all - it's set unconditionally by RecordPriceCheck
// regardless of whether that check found a price, so CheckedAt being
// non-nil with PriceCents nil is exactly "checked, nothing available" -
// the distinction a caller needs to tell "out of stock" apart from
// "haven't looked yet".
type MarketPrice struct {
	PriceCents *int
	CheckedAt  *time.Time
}

// GetLatestMarketPricesForSet returns market-price standing for every
// card in setID that has a card_price_sources row at all, keyed by
// card_id - a card with no source configured yet is simply absent from
// the map (there's nothing to report one way or the other), same as
// before this distinguished "checked" from "never checked". A card IS
// present, with CheckedAt set and PriceCents nil, when it's been checked
// but the source reported nothing (out of stock, delisted, or anything
// else) - this deliberately doesn't fall back to an earlier, stale price
// just because a more recent null exists; card_price_history's own doc
// comment covers why a null price is itself meaningful, not a gap to
// paper over.
//
// This is shared, catalog-level data - not scoped to any one user, same as
// ListCardsBySet itself - so it has no userID parameter.
func (p *Persist) GetLatestMarketPricesForSet(ctx context.Context, setID string) (map[string]MarketPrice, error) {
	// A window function picks each card's single most recent history row
	// (by recorded_at) in one query, rather than one query per card or a
	// less precise GROUP BY MAX(recorded_at) that can't also select
	// price_cents from that same row. Starting FROM card_price_sources
	// (not cards) is what makes a card with no source row at all simply
	// not appear below - only cards with something configured are
	// candidates for "checked" at all.
	rows, err := p.DB.QueryContext(ctx, `
		SELECT
			card_price_sources.card_id,
			card_price_sources.last_checked_at,
			latest.price_cents
		FROM card_price_sources
		JOIN cards ON cards.id = card_price_sources.card_id
		LEFT JOIN (
			SELECT
				card_id,
				price_cents,
				ROW_NUMBER() OVER (
					PARTITION BY card_id
					ORDER BY recorded_at DESC
				) AS rn
			FROM card_price_history
		) latest ON latest.card_id = card_price_sources.card_id AND latest.rn = 1
		WHERE cards.set_id = ?
	`, setID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing rows")
		}
	}()

	prices := make(map[string]MarketPrice)
	for rows.Next() {
		var cardID string
		var checkedAt sql.NullTime
		var priceCents sql.NullInt64
		if err := rows.Scan(&cardID, &checkedAt, &priceCents); err != nil {
			return nil, err
		}

		mp := MarketPrice{}
		if checkedAt.Valid {
			mp.CheckedAt = &checkedAt.Time
		}
		if priceCents.Valid {
			cents := int(priceCents.Int64)
			mp.PriceCents = &cents
		}
		prices[cardID] = mp
	}

	return prices, rows.Err()
}

// RecordPriceCheck records the outcome of checking cardID's price source -
// a new card_price_history row (priceCents nil when the card wasn't found
// on the page at all, i.e. no price to record) and an updated
// last_checked_at on card_price_sources, in one transaction so a crash
// between the two can't leave last_checked_at stale relative to history
// (or vice versa).
//
// last_checked_at is updated unconditionally, success or "not found" alike -
// a card whose page temporarily doesn't list it shouldn't get re-checked
// on every sync pass ahead of everything else's actual schedule, the same
// reasoning card_price_sources's own doc comment gives for source/url
// upserts never touching this column.
func (p *Persist) RecordPriceCheck(ctx context.Context, cardID, source string, priceCents *int) error {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && rollbackErr.Error() != "sql: transaction has already been committed or rolled back" {
			log.Error().Err(rollbackErr).Msg("error rolling back price check transaction")
		}
	}()

	_, err = sq.Insert("card_price_history").
		Columns("card_id", "source", "price_cents").
		Values(cardID, source, priceCents).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return err
	}

	_, err = sq.Update("card_price_sources").
		Set("last_checked_at", sq.Expr("NOW()")).
		Where(sq.Eq{"card_id": cardID}).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return err
	}

	return tx.Commit()
}
