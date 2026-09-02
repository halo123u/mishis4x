package ebay

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListingsCache_GetSet(t *testing.T) {
	c := newListingsCache(3, time.Hour)

	_, ok := c.get("card-1")
	require.False(t, ok, "nothing cached yet")

	want := []Listing{{ItemID: "v1|1|0", Title: "Test Listing"}}
	c.set("card-1", want)

	got, ok := c.get("card-1")
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestListingsCache_ExpiresAfterTTL(t *testing.T) {
	c := newListingsCache(3, 10*time.Millisecond)

	c.set("card-1", []Listing{{ItemID: "v1|1|0"}})
	_, ok := c.get("card-1")
	require.True(t, ok, "should be fresh immediately after set")

	time.Sleep(20 * time.Millisecond)
	_, ok = c.get("card-1")
	require.False(t, ok, "should be treated as a miss once past ttl")
}

func TestListingsCache_EvictsLeastRecentlyUsed(t *testing.T) {
	c := newListingsCache(2, time.Hour)

	c.set("card-1", []Listing{{ItemID: "1"}})
	c.set("card-2", []Listing{{ItemID: "2"}})
	// Touch card-1 so card-2 becomes the least-recently-used one.
	_, _ = c.get("card-1")

	c.set("card-3", []Listing{{ItemID: "3"}}) // pushes capacity=2 over the edge

	_, ok := c.get("card-2")
	require.False(t, ok, "card-2 was least-recently-used and should have been evicted")

	_, ok = c.get("card-1")
	require.True(t, ok, "card-1 was touched more recently and should survive")

	_, ok = c.get("card-3")
	require.True(t, ok, "the just-added card-3 should be present")
}

func TestListingsCache_SetOnExistingKeyRefreshesInPlace(t *testing.T) {
	c := newListingsCache(2, time.Hour)

	c.set("card-1", []Listing{{ItemID: "old"}})
	c.set("card-2", []Listing{{ItemID: "2"}})
	// card-1 is now the least-recently-used of the two.

	// Updating card-1 should count as a fresh touch, moving it back ahead
	// of card-2 - not just changing its value in place.
	c.set("card-1", []Listing{{ItemID: "new"}})

	c.set("card-3", []Listing{{ItemID: "3"}}) // pushes capacity=2 over the edge

	got, ok := c.get("card-1")
	require.True(t, ok, "card-1 was refreshed via set and should have moved to the front, surviving eviction")
	require.Equal(t, "new", got[0].ItemID)

	_, ok = c.get("card-2")
	require.False(t, ok, "card-2 wasn't touched since being added and should be the one evicted instead")

	_, ok = c.get("card-3")
	require.True(t, ok)
}
