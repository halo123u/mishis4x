package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"example.com/mishis4x/persist"
	"github.com/stretchr/testify/require"
)

func TestGetPriceTrends_Success(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithPriceTrends(t, db)
	createAndLoginTestUser(t, db, client, ts.URL)

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Price Trends Handler Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM card_price_history WHERE card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Trend Test Card", "TST/001", "SR")
	require.NoError(t, err)

	now := time.Now()
	dayAt := func(daysAgo, hour int) time.Time {
		d := now.AddDate(0, 0, -daysAgo)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, d.Location())
	}
	_, err = db.Exec(
		"INSERT INTO card_price_history (card_id, source, price_cents, recorded_at) VALUES (?, 'tcg_republic', ?, ?)",
		cardID, 1000, dayAt(3, 12),
	)
	require.NoError(t, err)
	_, err = db.Exec(
		"INSERT INTO card_price_history (card_id, source, price_cents, recorded_at) VALUES (?, 'tcg_republic', ?, ?)",
		cardID, 1200, dayAt(1, 12),
	)
	require.NoError(t, err)

	res, err := client.Get(ts.URL + "/api/sets/" + setID + "/price-trends")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var trends []struct {
		CardID        string  `json:"card_id"`
		ChangeCents   int     `json:"change_cents"`
		ChangePercent float64 `json:"change_percent"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&trends))
	require.Len(t, trends, 1)
	require.Equal(t, cardID, trends[0].CardID)
	require.Equal(t, 200, trends[0].ChangeCents)
}

func TestGetPriceTrends_SetNotFound(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithPriceTrends(t, db)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/sets/does-not-exist/price-trends")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestGetPriceTrends_DisabledByDefault(t *testing.T) {
	db := testDB(t)
	// newTestServer (not newTestServerWithPriceTrends) - PriceTrendsEnabled
	// defaults false, matching production until it's explicitly turned on.
	ts, client := newTestServer(t, db)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/sets/anything/price-trends")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
}

func TestGetPriceTrends_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithPriceTrends(t, db)

	res, err := http.Get(ts.URL + "/api/sets/anything/price-trends")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}
