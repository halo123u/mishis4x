package persist

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetPriceSourceForCard(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	ctx := t.Context()

	setID, err := p.CreateSet(ctx, "Price Source Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM card_price_sources WHERE card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(ctx, setID, "Test Card", "TST/001", "SR")
	require.NoError(t, err)

	t.Run("no source configured yet", func(t *testing.T) {
		_, _, found, err := p.GetPriceSourceForCard(ctx, cardID)
		require.NoError(t, err)
		require.False(t, found)
	})

	t.Run("source configured", func(t *testing.T) {
		require.NoError(t, p.UpsertPriceSource(ctx, cardID, "tcg_republic", "https://example.com/listing"))

		source, url, found, err := p.GetPriceSourceForCard(ctx, cardID)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, "tcg_republic", source)
		require.Equal(t, "https://example.com/listing", url)
	})
}

func TestGetLatestMarketPricesForSet_LastKnownPriceWhenOutOfStock(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	ctx := t.Context()

	setID, err := p.CreateSet(ctx, "Last Known Price Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM card_price_history WHERE card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID)
		_, _ = db.Exec("DELETE FROM card_price_sources WHERE card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(ctx, setID, "Test Card", "TST/002", "SR")
	require.NoError(t, err)
	require.NoError(t, p.UpsertPriceSource(ctx, cardID, "tcg_republic", "https://example.com/listing"))

	t.Run("never had a real price - both nil", func(t *testing.T) {
		require.NoError(t, p.RecordPriceCheck(ctx, cardID, "tcg_republic", nil))

		prices, err := p.GetLatestMarketPricesForSet(ctx, setID)
		require.NoError(t, err)
		got, ok := prices[cardID]
		require.True(t, ok)
		require.Nil(t, got.PriceCents)
		require.Nil(t, got.LastKnownPriceCents, "nothing was ever in stock, so there's no last-known price to report")
		require.Nil(t, got.LastKnownAt)
	})

	t.Run("in stock - last known matches the current price", func(t *testing.T) {
		priceCents := 500
		require.NoError(t, p.RecordPriceCheck(ctx, cardID, "tcg_republic", &priceCents))

		prices, err := p.GetLatestMarketPricesForSet(ctx, setID)
		require.NoError(t, err)
		got, ok := prices[cardID]
		require.True(t, ok)
		require.NotNil(t, got.PriceCents)
		require.Equal(t, 500, *got.PriceCents)
		require.NotNil(t, got.LastKnownPriceCents)
		require.Equal(t, 500, *got.LastKnownPriceCents)
	})

	t.Run("goes out of stock - last known survives as the earlier real price", func(t *testing.T) {
		require.NoError(t, p.RecordPriceCheck(ctx, cardID, "tcg_republic", nil))

		prices, err := p.GetLatestMarketPricesForSet(ctx, setID)
		require.NoError(t, err)
		got, ok := prices[cardID]
		require.True(t, ok)
		require.Nil(t, got.PriceCents, "the current check found nothing - still out of stock, not papered over")
		require.NotNil(t, got.CheckedAt)
		require.NotNil(t, got.LastKnownPriceCents, "the $5.00 seen one check ago must still surface as the last known price")
		require.Equal(t, 500, *got.LastKnownPriceCents)
		require.NotNil(t, got.LastKnownAt)
	})
}

func TestGetCardSearchInfo(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	ctx := t.Context()

	setID, err := p.CreateSet(ctx, "Card Search Info Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(ctx, setID, "Test Card", "BRD/W139-086S", "SR")
	require.NoError(t, err)

	t.Run("existing card", func(t *testing.T) {
		code, setName, found, err := p.GetCardSearchInfo(ctx, cardID)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, "BRD/W139-086S", code)
		require.Equal(t, "Card Search Info Test Set", setName)
	})

	t.Run("card does not exist", func(t *testing.T) {
		_, _, found, err := p.GetCardSearchInfo(ctx, "01900000-0000-7000-8000-000000000000")
		require.NoError(t, err)
		require.False(t, found)
	})
}
