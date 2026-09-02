package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/mishis4x/persist"
	"example.com/mishis4x/pricesync"
	"github.com/stretchr/testify/require"
)

// fixtureListingHTML mirrors the minimal shape pricesync.FetchTCGRepublicListing
// actually parses - a single real grid item with a price, no ranking widget
// needed for these tests.
const fixtureListingHTML = `<html><body>
<ul id="main_container" class="card_container">
<li class="product_thumbnail" style="width: 150px;">
	<a class="thumbnail_card   " href="/product/product_page_1.html">
		<div class="product_thumbnail_image">
			<img alt="Test Card BRD/W139-999S SR Foil" src="/media/x.jpg" />
			<div class="price_color_thumb">
				<span class="price_with_unit_offscreen">42.00</span>
			</div>
		</div>
	</a>
</li>
</ul>
</body></html>`

func TestRefreshCardPrice_Success(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	createAndLoginTestUser(t, db, client, ts.URL)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fixtureListingHTML))
	}))
	defer server.Close()

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Refresh Price Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM card_price_history WHERE card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID)
		_, _ = db.Exec("DELETE FROM card_price_sources WHERE card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Test Card", "BRD/W139-999S", "SR")
	require.NoError(t, err)
	require.NoError(t, p.UpsertPriceSource(t.Context(), cardID, "tcg_republic", server.URL))

	res, err := client.Post(ts.URL+"/api/cards/"+cardID+"/refresh-price", "", nil)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusNoContent, res.StatusCode)

	prices, err := p.GetLatestMarketPricesForSet(t.Context(), setID)
	require.NoError(t, err)
	got, ok := prices[cardID]
	require.True(t, ok)
	require.NotNil(t, got.PriceCents)
	require.Equal(t, 4200, *got.PriceCents)
	require.NotNil(t, got.CheckedAt)
}

func TestRefreshCardPrice_NoSourceConfigured(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	createAndLoginTestUser(t, db, client, ts.URL)

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Refresh Price Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Untracked Card", "BRD/W139-998S", "SR")
	require.NoError(t, err)

	res, err := client.Post(ts.URL+"/api/cards/"+cardID+"/refresh-price", "", nil)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestRefreshCardPrice_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	res, err := client.Post(ts.URL+"/api/cards/anything/refresh-price", "", nil)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

// TestRefreshCardPrice_RateLimited proves the endpoint actually surfaces
// pricesync.ErrRateLimited as 429, not a generic 500/502 - drains the
// shared pricesync.Limiter first so SyncURL has no token available.
func TestRefreshCardPrice_RateLimited(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	createAndLoginTestUser(t, db, client, ts.URL)

	originalLimiter := pricesync.Limiter
	pricesync.Limiter = pricesync.NewRateLimiter(0, time.Hour)
	t.Cleanup(func() { pricesync.Limiter = originalLimiter })

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Refresh Price Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM card_price_sources WHERE card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Rate Limited Card", "BRD/W139-997S", "SR")
	require.NoError(t, err)
	require.NoError(t, p.UpsertPriceSource(t.Context(), cardID, "tcg_republic", "https://example.com/unused"))

	res, err := client.Post(ts.URL+"/api/cards/"+cardID+"/refresh-price", "", nil)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusTooManyRequests, res.StatusCode)
}
