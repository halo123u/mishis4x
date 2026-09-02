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
