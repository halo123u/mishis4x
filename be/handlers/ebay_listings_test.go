package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/mishis4x/api"
	"example.com/mishis4x/ebay"
	"example.com/mishis4x/persist"
	"github.com/stretchr/testify/require"
)

// fakeEbayServer mirrors ebay package's own test fixture - a fixed oauth
// token plus a small, real-shaped item_summary/search response - kept as
// its own small copy here rather than exported from the ebay package
// purely for a test fixture.
func fakeEbayServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 7200})
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"itemSummaries": []map[string]any{
				{
					"itemId":     "v1|111|0",
					"title":      "Weiss Schwarz Brown Dust 2 BRD/W139-086S",
					"price":      map[string]any{"value": "42.00", "currency": "USD"},
					"condition":  "New",
					"seller":     map[string]any{"username": "seller_a", "feedbackPercentage": "99.5"},
					"itemWebUrl": "https://www.ebay.com/itm/111",
					"image":      map[string]any{"imageUrl": "https://i.ebayimg.com/1.jpg"},
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestGetEbayListings_Success(t *testing.T) {
	db := testDB(t)
	fake := fakeEbayServer(t)
	svc := ebay.NewServiceWithURLs("app-id", "cert-id", fake.URL+"/token", fake.URL+"/search")
	ts, client := newTestServerWithEbay(t, db, svc)
	createAndLoginTestUser(t, db, client, ts.URL)

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Ebay Listings Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Test Card", "BRD/W139-086S", "SR")
	require.NoError(t, err)

	res, err := client.Get(ts.URL + "/api/cards/" + cardID + "/ebay-listings")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var data api.EbayListingsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&data))
	require.Equal(t, "Ebay Listings Test Set 086S", data.Query)
	require.Len(t, data.Listings, 1)
	require.Equal(t, "v1|111|0", data.Listings[0].ItemID)
	require.Equal(t, 4200, data.Listings[0].PriceCents)
	require.Equal(t, "seller_a", data.Listings[0].SellerUsername)
}

func TestGetEbayListings_CardNotFound(t *testing.T) {
	db := testDB(t)
	fake := fakeEbayServer(t)
	svc := ebay.NewServiceWithURLs("app-id", "cert-id", fake.URL+"/token", fake.URL+"/search")
	ts, client := newTestServerWithEbay(t, db, svc)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/cards/01900000-0000-7000-8000-000000000000/ebay-listings")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

// TestGetEbayListings_NotConfigured proves the endpoint reports a clean
// 503 rather than a panic/500 when Data.Ebay is nil (no credentials
// configured) - the common case for any environment that hasn't set up
// eBay yet.
func TestGetEbayListings_NotConfigured(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db) // no ebay.Service wired
	createAndLoginTestUser(t, db, client, ts.URL)

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Ebay Listings Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Test Card", "BRD/W139-086S", "SR")
	require.NoError(t, err)

	res, err := client.Get(ts.URL + "/api/cards/" + cardID + "/ebay-listings")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
}

func TestGetEbayListings_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	res, err := client.Get(ts.URL + "/api/cards/anything/ebay-listings")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}
