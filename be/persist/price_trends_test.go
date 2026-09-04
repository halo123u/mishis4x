package persist

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetPriceTrendsForSet(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	ctx := t.Context()

	setID, err := p.CreateSet(ctx, "Price Trends Test Set", 3, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM card_price_history WHERE card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	trendingCard, err := p.CreateCard(ctx, setID, "Trending Card", "TST/001", "SR")
	require.NoError(t, err)
	oneCheckCard, err := p.CreateCard(ctx, setID, "One Check Card", "TST/002", "SR")
	require.NoError(t, err)
	noChecksCard, err := p.CreateCard(ctx, setID, "No Checks Card", "TST/003", "SR")
	require.NoError(t, err)

	now := time.Now()
	insert := func(cardID, source string, priceCents *int, recordedAt time.Time) {
		t.Helper()
		_, err := db.Exec(
			"INSERT INTO card_price_history (card_id, source, price_cents, recorded_at) VALUES (?, ?, ?, ?)",
			cardID, source, priceCents, recordedAt,
		)
		require.NoError(t, err)
	}
	cents := func(v int) *int { return &v }

	// dayAt builds an unambiguous timestamp daysAgo calendar days back at a
	// fixed hour - using raw now.Add(-N*24h) arithmetic for the same-day-
	// duplicate case below would be flaky depending on what time of day
	// the test happens to run (a few hours' difference can land on either
	// side of a midnight boundary), so this pins the calendar date
	// explicitly and only varies the hour within it. Callers should still
	// keep hour comfortably mid-day (confirmed the hard way) - the DB
	// connection/session timezone can shift a value by several hours
	// between construction and what DATE(recorded_at) sees, so an hour
	// chosen near midnight can still land on the wrong calendar day.
	dayAt := func(daysAgo, hour int) time.Time {
		d := now.AddDate(0, 0, -daysAgo)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, d.Location())
	}

	// trendingCard: 3 days within the window, plus a same-day duplicate
	// (only the later one should count) and an eBay-sourced row (must be
	// ignored) and a too-old row (outside the 7-day window, must be
	// ignored).
	insert(trendingCard, "tcg_republic", cents(1000), dayAt(6, 12))
	insert(trendingCard, "tcg_republic", cents(1100), dayAt(3, 12))
	// Both comfortably mid-day (not near a midnight boundary) - a
	// timezone shift between how this test constructs the time and how
	// the DB stores/reports it (see dayAt's own doc comment) could
	// otherwise push an early- or late-hour timestamp into a different
	// calendar day than intended.
	insert(trendingCard, "tcg_republic", cents(1180), dayAt(1, 10)) // same day, earlier check
	insert(trendingCard, "tcg_republic", cents(1200), dayAt(1, 14)) // same day, later check
	insert(trendingCard, "ebay", cents(99999), dayAt(2, 12))        // wrong source
	insert(trendingCard, "tcg_republic", cents(1), dayAt(10, 12))   // outside window

	// oneCheckCard: only ever checked once - nothing to compare against.
	insert(oneCheckCard, "tcg_republic", cents(500), dayAt(2, 12))

	// noChecksCard: no rows at all.

	trends, err := p.GetPriceTrendsForSet(ctx, setID)
	require.NoError(t, err)

	require.NotContains(t, trends, oneCheckCard, "a single check has nothing to compare against")
	require.NotContains(t, trends, noChecksCard)

	trend, ok := trends[trendingCard]
	require.True(t, ok)
	require.Len(t, trend.DailyPrices, 3, "3 distinct days - the same-day duplicate collapses to its later check, the eBay row and the too-old row are both excluded")
	require.Equal(t, 1000, trend.DailyPrices[0].PriceCents)
	require.Equal(t, 1100, trend.DailyPrices[1].PriceCents)
	require.Equal(t, 1200, trend.DailyPrices[2].PriceCents, "the later same-day check (1200) should win over the earlier one (1180)")
	require.Equal(t, 200, trend.ChangeCents)
	require.InDelta(t, 20.0, trend.ChangePercent, 0.01)
}

func TestGetPriceTrendsForSet_NoData(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	ctx := t.Context()

	setID, err := p.CreateSet(ctx, "Price Trends Empty Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	_, err = p.CreateCard(ctx, setID, "Untracked Card", "TST/001", "SR")
	require.NoError(t, err)

	trends, err := p.GetPriceTrendsForSet(ctx, setID)
	require.NoError(t, err)
	require.Empty(t, trends)
}
