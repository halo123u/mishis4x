package pricesync

import (
	"sync"
	"time"
)

// Limiter is the process-wide token bucket every SyncURL call draws from -
// shared between the background 12h sync loop (cmd/http.go's
// runPriceSyncLoop, via SyncAll) and the on-demand per-card refresh
// endpoint (be/handlers.RefreshCardPrice), since both ultimately hit the
// same external site (TCG Republic) and need to draw from the same budget
// rather than each pacing itself independently - two things that are each
// individually polite can still combine into a burst neither one alone
// would produce.
//
// capacity=10, refilling 1 token every 30s: generous enough that the
// background loop's handful of distinct urls (already paced SyncDelay
// apart) essentially never sees an empty bucket, while a burst of manual
// refresh clicks still gets capped rather than hitting TCG Republic once
// per click with no limit at all.
var Limiter = NewRateLimiter(10, 30*time.Second)

// RateLimiter is a simple token bucket - lazily refilled on each
// TryAcquire call based on elapsed time, rather than a background
// goroutine/ticker keeping it topped up.
//
// In-memory, single-instance - same tradeoff as attemptLimiter (see
// be/handlers/attemptLimiter.go's doc comment): resets on restart, no
// cross-instance coordination. Acceptable given this app runs as a single
// instance and the goal here is "don't let a burst of manual clicks
// hammer a third-party site," not a hard distributed guarantee.
type RateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	refillRate float64 // tokens per second
	lastCheck  time.Time
}

// NewRateLimiter builds a limiter that starts full (capacity tokens
// available immediately) and refills at one token per refillInterval.
func NewRateLimiter(capacity int, refillInterval time.Duration) *RateLimiter {
	return &RateLimiter{
		tokens:     float64(capacity),
		capacity:   float64(capacity),
		refillRate: 1 / refillInterval.Seconds(),
		lastCheck:  time.Now(),
	}
}

// TryAcquire takes one token if one's available right now, reporting
// whether it did. Never blocks - a caller that can't get a token decides
// for itself what that means (the background loop just skips that url
// until its next cycle; the on-demand endpoint returns 429 immediately
// rather than making the caller wait).
func (r *RateLimiter) TryAcquire() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastCheck).Seconds()
	r.lastCheck = now
	r.tokens = min(r.capacity, r.tokens+elapsed*r.refillRate)

	if r.tokens < 1 {
		return false
	}
	r.tokens--
	return true
}
