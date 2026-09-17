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

func TestGetOwnedCardPriceMovers(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	ctx := t.Context()
	userID := setupOwnershipTestUser(t, p)

	setID, err := p.CreateSet(ctx, "Price Movers Test Set", 4, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_card_copies WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM card_price_history WHERE card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	gainerCard, err := p.CreateCard(ctx, setID, "Gainer Card", "TST/001", "SR")
	require.NoError(t, err)
	loserCard, err := p.CreateCard(ctx, setID, "Loser Card", "TST/002", "SR")
	require.NoError(t, err)
	oneCheckOwnedCard, err := p.CreateCard(ctx, setID, "One Check Owned Card", "TST/003", "SR")
	require.NoError(t, err)
	unownedCard, err := p.CreateCard(ctx, setID, "Unowned Card", "TST/004", "SR")
	require.NoError(t, err)

	require.NoError(t, p.SetOwnedCards(ctx, userID, []CardQuantity{
		{CardID: gainerCard, Quantity: 1},
		{CardID: loserCard, Quantity: 1},
		{CardID: oneCheckOwnedCard, Quantity: 1},
		// unownedCard deliberately not included.
	}))

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
	// Same reasoning/pitfalls as TestGetPriceTrendsForSet's identical
	// helper - see its own doc comment.
	dayAt := func(daysAgo, hour int) time.Time {
		d := now.AddDate(0, 0, -daysAgo)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, d.Location())
	}

	// gainerCard: 3 days of history - only the latest 2 (day_rank 1/2)
	// should ever be compared, the oldest (dayAt(5,...)) is a distractor
	// that would give the wrong answer if the query used first-vs-last
	// like GetPriceTrendsForSet does instead of latest-two.
	insert(gainerCard, "tcg_republic", cents(5000), dayAt(5, 12))
	insert(gainerCard, "tcg_republic", cents(1000), dayAt(2, 12))
	insert(gainerCard, "tcg_republic", cents(1200), dayAt(1, 12))

	// loserCard: same-day duplicate on its latest day (only the later
	// check should count) and a wrong-source row that must be ignored.
	insert(loserCard, "tcg_republic", cents(2000), dayAt(3, 12))
	insert(loserCard, "tcg_republic", cents(1850), dayAt(1, 10)) // same day, earlier
	insert(loserCard, "tcg_republic", cents(1800), dayAt(1, 14)) // same day, later - should win
	insert(loserCard, "ebay", cents(99999), dayAt(1, 12))        // wrong source

	// oneCheckOwnedCard: owned, but only ever checked once - nothing to
	// diff against, must not appear in the result at all.
	insert(oneCheckOwnedCard, "tcg_republic", cents(500), dayAt(1, 12))

	// unownedCard: real 2-day history, but never owned - must never
	// appear regardless of how much its price moved.
	insert(unownedCard, "tcg_republic", cents(100), dayAt(2, 12))
	insert(unownedCard, "tcg_republic", cents(9999), dayAt(1, 12))

	movers, err := p.GetOwnedCardPriceMovers(ctx, userID)
	require.NoError(t, err)
	require.Len(t, movers, 2, "oneCheckOwnedCard (1 day) and unownedCard (not owned) must both be excluded")

	// Sorted by |ChangeCents| descending, but gainerCard's swing (+200)
	// and loserCard's (-200) tie in magnitude here, so which one sorts
	// first isn't guaranteed - assert on membership/values by CardID
	// rather than positional order.
	byCardID := map[string]CardPriceMover{}
	for _, m := range movers {
		byCardID[m.CardID] = m
	}

	gainer, ok := byCardID[gainerCard]
	require.True(t, ok)
	require.Equal(t, setID, gainer.SetID)
	require.Equal(t, "Gainer Card", gainer.Name)
	require.Equal(t, 1000, gainer.PreviousCents)
	require.Equal(t, 1200, gainer.LatestCents)
	require.Equal(t, 200, gainer.ChangeCents)
	require.InDelta(t, 20.0, gainer.ChangePercent, 0.01)

	loser, ok := byCardID[loserCard]
	require.True(t, ok)
	require.Equal(t, 2000, loser.PreviousCents)
	require.Equal(t, 1800, loser.LatestCents, "the later same-day check (1800) should win over the earlier one (1850)")
	require.Equal(t, -200, loser.ChangeCents)
	require.InDelta(t, -10.0, loser.ChangePercent, 0.01)
}

func TestGetOwnedCardPriceMovers_NoData(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	ctx := t.Context()
	userID := setupOwnershipTestUser(t, p)

	movers, err := p.GetOwnedCardPriceMovers(ctx, userID)
	require.NoError(t, err)
	require.Empty(t, movers)
}
