package cmd

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"example.com/mishis4x/persist"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// testDB mirrors persist's and handlers' own testDB helpers - skips
// (t.Skip, not a failure) when no DB is reachable, same tolerance so
// `go test ./...` still works standalone.
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

func TestParsePort(t *testing.T) {
	port, err := parsePort("")
	require.NoError(t, err)
	require.Equal(t, defaultPort, port)

	port, err = parsePort("9090")
	require.NoError(t, err)
	require.Equal(t, 9090, port)

	_, err = parsePort("not-a-number")
	require.Error(t, err)

	_, err = parsePort("0")
	require.Error(t, err)

	_, err = parsePort("-1")
	require.Error(t, err)

	_, err = parsePort("70000")
	require.Error(t, err)
}

// TestRunPriceSyncLoop_TicksAndStopsOnCancel exercises the loop's actual
// concurrency behavior - the part worth testing here, not pricesync.SyncAll
// itself (that's persist/pricesync's job). A short interval lets several
// ticks fire safely within the test window: a fresh test DB has no
// card_price_sources rows at all, so each tick's SyncAll call finds zero
// urls and returns near-instantly, with no real network activity.
// Confirms the loop doesn't hang or leak - it must exit promptly once ctx
// is canceled, not linger until its next tick.
func TestRunPriceSyncLoop_TicksAndStopsOnCancel(t *testing.T) {
	db := testDB(t)
	p := &persist.Persist{DB: db}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runPriceSyncLoop(ctx, p, 20*time.Millisecond)
		close(done)
	}()

	// Long enough for several ticks to have fired at the 20ms interval.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runPriceSyncLoop did not stop within 2s of context cancellation")
	}
}
