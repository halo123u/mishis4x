package ebay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearchQuery(t *testing.T) {
	tests := []struct {
		name    string
		setName string
		code    string
		want    string
	}{
		{"set name and full code", "Brown Dust 2", "BRD/W139-086S", "Brown Dust 2 086S"},
		{"code with no dash - falls back to the whole thing", "Brown Dust 2", "086S", "Brown Dust 2 086S"},
		{"no set name - just the short code", "", "BRD/W139-086S", "086S"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, SearchQuery(tt.setName, tt.code))
		})
	}
}

func TestParsePriceCents(t *testing.T) {
	tests := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"42.00", 4200, false},
		{"5", 500, false},
		{"5.5", 550, false},
		{"0.05", 5, false},
		{"not-a-number", 0, true},
	}
	for _, tt := range tests {
		got, err := parsePriceCents(tt.in)
		if tt.wantErr {
			require.Error(t, err, "input %q", tt.in)
			continue
		}
		require.NoError(t, err, "input %q", tt.in)
		require.Equal(t, tt.want, got, "input %q", tt.in)
	}
}

// fakeEbayServer stands in for both eBay's oauth token endpoint and its
// Browse API search endpoint, on one httptest.Server (tests don't need
// them on separate hosts) - returns a fixed token and a small, real-shaped
// item_summary/search response.
func fakeEbayServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fake-token",
			"expires_in":   7200,
		})
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer fake-token", r.Header.Get("Authorization"))
		require.Equal(t, "EBAY_US", r.Header.Get("X-EBAY-C-MARKETPLACE-ID"))

		q, err := url.QueryUnescape(r.URL.Query().Get("q"))
		require.NoError(t, err)
		require.Equal(t, "Brown Dust 2 086S", q)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"itemSummaries": []map[string]any{
				{
					"itemId":    "v1|111|0",
					"title":     "Weiss Schwarz Brown Dust 2 BRD/W139-086S",
					"price":     map[string]any{"value": "42.00", "currency": "USD"},
					"condition": "New",
					"seller": map[string]any{
						"username":           "seller_a",
						"feedbackPercentage": "99.5",
					},
					"itemWebUrl": "https://www.ebay.com/itm/111",
					"image":      map[string]any{"imageUrl": "https://i.ebayimg.com/1.jpg"},
				},
				{
					"itemId":    "v1|222|0",
					"title":     "BRD/W139-086S SR foil",
					"price":     map[string]any{"value": "48.50", "currency": "USD"},
					"condition": "Used",
					"seller": map[string]any{
						"username":           "seller_b",
						"feedbackPercentage": "100.0",
					},
					"itemWebUrl": "https://www.ebay.com/itm/222",
					"image":      map[string]any{"imageUrl": "https://i.ebayimg.com/2.jpg"},
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestService_GetListings_FetchesLiveThenCaches(t *testing.T) {
	server := fakeEbayServer(t)
	svc := NewServiceWithURLs("app-id", "cert-id", server.URL+"/token", server.URL+"/search")

	listings, err := svc.GetListings(t.Context(), "card-1", "Brown Dust 2 086S")
	require.NoError(t, err)
	require.Len(t, listings, 2)
	require.Equal(t, "v1|111|0", listings[0].ItemID)
	require.Equal(t, 4200, listings[0].PriceCents)
	require.Equal(t, "seller_a", listings[0].SellerUsername)
	require.Equal(t, 4850, listings[1].PriceCents)

	// The listingsCache-level tests already prove get/set/eviction/TTL
	// behavior in isolation - this just confirms GetListings actually
	// wrote through to the cache after a live fetch, not that a second
	// call skips the network (that's the cache's job, already covered).
	svc.cache.mu.Lock()
	_, cached := svc.cache.items["card-1"]
	svc.cache.mu.Unlock()
	require.True(t, cached)
}

func TestService_GetListings_UnsupportedStatusIsAnError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 7200})
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	svc := NewServiceWithURLs("app-id", "cert-id", server.URL+"/token", server.URL+"/search")

	_, err := svc.GetListings(t.Context(), "card-1", "anything")
	require.Error(t, err)
}
