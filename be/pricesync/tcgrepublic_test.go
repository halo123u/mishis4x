package pricesync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePriceCents(t *testing.T) {
	tests := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"200.00", 20000, false},
		{"1125.00", 112500, false},
		{"5", 500, false},   // no decimal point - treated as whole dollars
		{"5.5", 550, false}, // single-digit cents padded, not misread as 5 cents
		{"0.05", 5, false},  // leading-zero cents preserved
		// Both of these are realistic error inputs given how this is
		// actually called - pricePattern's capture group is [0-9.]+, so a
		// malformed multi-dot capture (never a letter, that couldn't match
		// the regex at all) is the error case actually worth covering.
		{"1.2.3", 0, true},
		{"...", 0, true},
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

func TestMatchListingItem(t *testing.T) {
	items := []ListingItem{
		{Text: "Ranking Widget Card BRD/W139-999S SR Foil", PriceCents: 5000, IsRanking: true},
		{Text: "Real Grid Card BRD/W139-999S SR Foil", PriceCents: 20000, IsRanking: false},
		{Text: "Only In Ranking BRD/W139-050S SR Foil", PriceCents: 1500, IsRanking: true},
	}

	t.Run("prefers the non-ranking match when both exist for the same code", func(t *testing.T) {
		item, found := MatchListingItem(items, "BRD/W139-999S")
		require.True(t, found)
		require.Equal(t, 20000, item.PriceCents)
		require.False(t, item.IsRanking)
	})

	t.Run("falls back to a ranking-only match if that's the only one", func(t *testing.T) {
		item, found := MatchListingItem(items, "BRD/W139-050S")
		require.True(t, found)
		require.Equal(t, 1500, item.PriceCents)
		require.True(t, item.IsRanking)
	})

	t.Run("returns found=false for a code that isn't present at all", func(t *testing.T) {
		_, found := MatchListingItem(items, "BRD/W139-777S")
		require.False(t, found)
	})
}

// fixtureListingHTML mirrors real TCG Republic markup captured directly
// from a live category listing page this session (see
// FetchTCGRepublicListing's doc comment) - a "popular items" ranking
// widget block, a real grid item with a price, and a real grid item
// that's out of stock (no price span at all, just a "Not Available"
// marker) to confirm that one is correctly omitted rather than included
// with some inferred status.
const fixtureListingHTML = `<html><body>
<ul id="ranking_container">
<li class="product_thumbnail">
	<a class="thumbnail_card   popular_item" href="/product/product_page_1111111111.html?ref=category_page&type=ranking_product">
		<div class="product_thumbnail_image">
			<img alt="Ranking Widget Card BRD/W139-999S SR Foil" src="/media/x.jpg" />
			<div class="price_color_thumb">
				<span class="price_with_unit_offscreen">50.00</span>
			</div>
		</div>
	</a>
</li>
</ul>
<ul id="main_container" class="card_container">
<li class="product_thumbnail" style="width: 150px;">
	<a class="thumbnail_card   " href="/product/product_page_2000763578.html">
		<div class="product_thumbnail_image">
			<img alt="Real Grid Card BRD/W139-999S SR Foil" src="/media/y.jpg" />
			<div class="price_color_thumb">
				<span class="price_with_unit_offscreen">200.00</span>
			</div>
		</div>
	</a>
</li>
<li class="product_thumbnail" style="width: 150px;">
	<a class="thumbnail_card   " href="/product/product_page_2000763499.html">
		<div class="product_thumbnail_image">
			<img alt="Out Of Stock Card BRD/W139-010S SR Foil" src="/media/z.jpg" />
			<div class="price_color_thumb red">
				Not Available
			</div>
		</div>
	</a>
</li>
</ul>
</body></html>`

func TestFetchTCGRepublicListing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fixtureListingHTML))
	}))
	defer server.Close()

	items, err := FetchTCGRepublicListing(context.Background(), server.URL)
	require.NoError(t, err)

	// Exactly 2 items - the out-of-stock block has no price span, so it's
	// omitted entirely rather than included with an inferred status (see
	// card_price_history's own doc comment for why that's deliberate).
	require.Len(t, items, 2)

	rankingItem, found := MatchListingItem([]ListingItem{items[0]}, "BRD/W139-999S")
	require.True(t, found)
	require.Equal(t, 5000, rankingItem.PriceCents)
	require.True(t, rankingItem.IsRanking)

	realItem, found := MatchListingItem([]ListingItem{items[1]}, "BRD/W139-999S")
	require.True(t, found)
	require.Equal(t, 20000, realItem.PriceCents)
	require.False(t, realItem.IsRanking)

	_, found = MatchListingItem(items, "BRD/W139-010S")
	require.False(t, found, "the out-of-stock card should not appear in the parsed items at all")
}

func TestFetchTCGRepublicListing_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := FetchTCGRepublicListing(context.Background(), server.URL)
	require.Error(t, err)
}
