package ebay

import (
	"container/list"
	"sync"
	"time"
)

// cacheCapacity bounds how many distinct cards' listings this process
// holds onto at once - deliberately small (not "every card in the
// catalog") since a card someone hasn't checked recently just falls out
// and gets a fresh live fetch next time, same tradeoff any bounded cache
// makes. Evicted least-recently-used first once full.
const cacheCapacity = 30

// cacheTTL matches eBay's own API License Agreement staleness allowance
// for item listing data (6h) - see [[ebay-api-license-terms]] - so this
// cache never holds data longer than eBay actually permits.
const cacheTTL = 6 * time.Hour

type cacheEntry struct {
	cardID   string
	listings []Listing
	cachedAt time.Time
}

// listingsCache is a bounded, in-memory LRU cache of one card's most
// recent eBay listings, with entries treated as expired once older than
// cacheTTL. In-memory, single-instance - same tradeoff as attemptLimiter/
// pricesync.RateLimiter (see their doc comments): resets on restart, no
// cross-instance coordination, acceptable given this app runs as a single
// instance and a cache miss just means one extra live eBay call, not a
// correctness problem.
type listingsCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	order    *list.List
	items    map[string]*list.Element
}

func newListingsCache(capacity int, ttl time.Duration) *listingsCache {
	return &listingsCache{
		capacity: capacity,
		ttl:      ttl,
		order:    list.New(),
		items:    make(map[string]*list.Element),
	}
}

// get returns cardID's cached listings if present and not yet past ttl,
// moving it to the front of the LRU order on a hit. A stale entry is
// reported as a miss but left in place - a subsequent set overwrites it
// in place rather than needing a separate expiry sweep.
func (c *listingsCache) get(cardID string) ([]Listing, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[cardID]
	if !ok {
		return nil, false
	}

	entry := el.Value.(*cacheEntry)
	if time.Since(entry.cachedAt) > c.ttl {
		return nil, false
	}

	c.order.MoveToFront(el)
	return entry.listings, true
}

// set records cardID's listings as freshly fetched just now, evicting the
// single least-recently-used entry if this would push the cache past
// capacity. Updating an already-cached card moves it to the front rather
// than growing the cache.
func (c *listingsCache) set(cardID string, listings []Listing) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[cardID]; ok {
		c.order.MoveToFront(el)
		entry := el.Value.(*cacheEntry)
		entry.listings = listings
		entry.cachedAt = time.Now()
		return
	}

	el := c.order.PushFront(&cacheEntry{cardID: cardID, listings: listings, cachedAt: time.Now()})
	c.items[cardID] = el

	if c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*cacheEntry).cardID)
		}
	}
}
