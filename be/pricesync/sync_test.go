package pricesync

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"example.com/mishis4x/persist"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// testDB mirrors persist's, handlers', and cmd's own testDB helpers -
// skips (t.Skip, not a failure) when no DB is reachable, so `go test
// ./...` still works standalone.
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost:3306"
	}
	cfg := mysql.Config{
		User:                 envOr("DB_USERNAME", "root"),
		Passwd:               envOr("DB_PASSWORD", "root_password"),
		Net:                  "tcp",
		Addr:                 host,
		DBName:               envOr("DB_NAME", "mishis4x"),
		AllowNativePasswords: true,
		ParseTime:            true,
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Ping(); err != nil {
		t.Skipf("no test database reachable at %s, skipping integration test: %v", host, err)
	}

	return db
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// TestSyncAll_RecordsPricesAndMisses is the end-to-end proof that SyncAll
// actually works, not just that it prints something plausible-looking to
// a log: a real card_price_sources row pointing at a fake HTTP server (no
// real network dependency, deterministic, fast) gets a real
// card_price_history row written from what that server actually served,
// and a card whose code never appears on the page gets a real "checked,
// nothing found" row instead of silently nothing at all.
func TestSyncAll_RecordsPricesAndMisses(t *testing.T) {
	db := testDB(t)
	p := &persist.Persist{DB: db}
	ctx := t.Context()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fixtureListingHTML))
	}))
	defer server.Close()

	setID, err := p.CreateSet(ctx, "Sync Test Set", 2, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM card_price_history WHERE card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID)
		_, _ = db.Exec("DELETE FROM card_price_sources WHERE card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	// BRD/W139-999S is on the fixture page (see TestFetchTCGRepublicListing);
	// BRD/W139-777S deliberately isn't - both share the same source url,
	// same as how many cards share one real TCG Republic listing page.
	foundCardID, err := p.CreateCard(ctx, setID, "Found Card", "BRD/W139-999S", "SR")
	require.NoError(t, err)
	missingCardID, err := p.CreateCard(ctx, setID, "Missing Card", "BRD/W139-777S", "SR")
	require.NoError(t, err)

	require.NoError(t, p.UpsertPriceSource(ctx, foundCardID, "tcg_republic", server.URL))
	require.NoError(t, p.UpsertPriceSource(ctx, missingCardID, "tcg_republic", server.URL))

	stats, err := SyncAll(ctx, p)
	require.NoError(t, err)
	require.Equal(t, 2, stats.Checked)
	require.Equal(t, 1, stats.Matched)
	require.Equal(t, 1, stats.Unmatched)
	require.Equal(t, 0, stats.Errored)

	prices, err := p.GetLatestMarketPricesForSet(ctx, setID)
	require.NoError(t, err)

	found, ok := prices[foundCardID]
	require.True(t, ok)
	require.NotNil(t, found.PriceCents)
	// The real (non-ranking) grid entry's price, not the ranking widget's -
	// confirms MatchListingItem's dedup preference actually flows through
	// SyncAll end-to-end, not just in isolation.
	require.Equal(t, 20000, *found.PriceCents)
	require.NotNil(t, found.CheckedAt)

	missing, ok := prices[missingCardID]
	require.True(t, ok)
	require.Nil(t, missing.PriceCents)
	require.NotNil(t, missing.CheckedAt, "a card that's been checked but not found should still get last_checked_at set")
}

// withShortRetryDelay swaps fetchRetryDelay down to a couple of
// milliseconds for the duration of one test, so a test that deliberately
// exercises retries doesn't have to wait out several real seconds of
// backoff. Restores the real value on cleanup rather than leaving a
// shrunk delay bleeding into other tests in the package.
func withShortRetryDelay(t *testing.T) {
	t.Helper()
	original := fetchRetryDelay
	fetchRetryDelay = 2 * time.Millisecond
	t.Cleanup(func() { fetchRetryDelay = original })
}

// TestFetchListingWithRetry_SucceedsAfterTransientFailures proves a
// transient failure (the first couple of requests failing) doesn't sink
// the whole url - it should recover within the retry cap rather than
// getting logged as errored and left stale until the next scheduled sync.
func TestFetchListingWithRetry_SucceedsAfterTransientFailures(t *testing.T) {
	withShortRetryDelay(t)

	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(fixtureListingHTML))
	}))
	defer server.Close()

	items, err := fetchListingWithRetry(t.Context(), server.URL)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, int32(3), atomic.LoadInt32(&attempts), "should succeed on the 3rd attempt, within maxFetchAttempts")
}

// TestFetchListingWithRetry_GivesUpAfterMaxAttempts is the direct proof of
// the "cap" - a url that's genuinely broken (never succeeds) must still
// give up after exactly maxFetchAttempts tries and return an error,
// rather than retrying forever.
func TestFetchListingWithRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	withShortRetryDelay(t)

	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := fetchListingWithRetry(t.Context(), server.URL)
	require.Error(t, err)
	require.Equal(t, int32(maxFetchAttempts), atomic.LoadInt32(&attempts))
}

// TestFetchListingWithRetry_StopsOnContextCancellation confirms a
// cancellation during the backoff wait between attempts returns promptly
// with ctx.Err() rather than blocking out the rest of fetchRetryDelay.
func TestFetchListingWithRetry_StopsOnContextCancellation(t *testing.T) {
	original := fetchRetryDelay
	fetchRetryDelay = time.Hour // long enough that only cancellation ends the wait
	t.Cleanup(func() { fetchRetryDelay = original })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := fetchListingWithRetry(ctx, server.URL)
	require.ErrorIs(t, err, context.Canceled)
}
