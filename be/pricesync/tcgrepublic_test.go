package pricesync

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContainsCodeToken(t *testing.T) {
	tests := []struct {
		name string
		text string
		code string
		want bool
	}{
		{
			name: "exact code at end of string",
			text: "Celebrity Bunny Roen BRD/W139-054S",
			code: "BRD/W139-054S",
			want: true,
		},
		{
			name: "exact code followed by a space and more text",
			text: "Celebrity Bunny Roen BRD/W139-054S SR Foil",
			code: "BRD/W139-054S",
			want: true,
		},
		{
			name: "code is only a prefix of a longer variant's code - must not match",
			text: "Roen (Celebrity Gold) BRD/W139-054SSP SSP Foil",
			code: "BRD/W139-054S",
			want: false,
		},
		{
			name: "code not present at all",
			text: "Neblis BRD/W139-029S SR Foil",
			code: "BRD/W139-054S",
			want: false,
		},
		{
			name: "the longer code itself still matches correctly",
			text: "Roen (Celebrity Gold) BRD/W139-054SSP SSP Foil",
			code: "BRD/W139-054SSP",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, containsCodeToken(tt.text, tt.code))
		})
	}
}

// TestMatchListingItem_DoesNotConfuseVariantCodes is the direct regression
// test for a real bug: BRD/W139-054S and BRD/W139-029S were both found
// recording their SSP variant's price instead of their own, because a
// plain substring check treated the shorter code as "found" merely for
// being a literal prefix of the longer one whenever both happened to
// appear in the same fetched page (e.g. the SSP variant showing up in the
// repeated "popular items" ranking sidebar - see ListingItem.IsRanking).
func TestMatchListingItem_DoesNotConfuseVariantCodes(t *testing.T) {
	items := []ListingItem{
		{Text: "Celebrity Bunny Roen BRD/W139-054S SR Foil", PriceCents: 50000, IsRanking: false},
		{Text: "Roen (Celebrity Gold) BRD/W139-054SSP SSP Foil", PriceCents: 88888, IsRanking: true},
	}

	item, found := MatchListingItem(items, "BRD/W139-054S")
	require.True(t, found)
	require.Equal(t, 50000, item.PriceCents, "054S must get its own price, not its 054SSP variant's")

	item, found = MatchListingItem(items, "BRD/W139-054SSP")
	require.True(t, found)
	require.Equal(t, 88888, item.PriceCents)
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
