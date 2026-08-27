package handlers

import (
	"database/sql"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"example.com/mishis4x/matchmaking"
	"example.com/mishis4x/persist"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// testDB connects to a real MySQL instance for integration tests (see
// CLAUDE.md's testing philosophy) - skips (not fails) if none is reachable,
// so `go test ./...` still works without one; CI runs these for real.
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

// testSessionCookieConfig is a fixed, non-Secure config (tests run over
// plain HTTP via httptest) shared by every handler test that needs a full
// *Data.
func testSessionCookieConfig() SessionCookieConfig {
	return SessionCookieConfig{
		Name:   "session",
		Secure: false,
		TTL:    time.Hour,
	}
}

// newTestData builds a real, fully-wired Data (real DB, real rate limiter)
// for tests to spin up a router from. CollectionOwnerUserID is 0 (nobody) -
// tests that need collection-tracker routes to actually authorize someone
// should use newTestDataWithOwner instead.
func newTestData(db *sql.DB) *Data {
	return newTestDataWithOwner(db, 0)
}

func newTestDataWithOwner(db *sql.DB, ownerUserID int) *Data {
	return NewData(
		persist.Persist{DB: db},
		&matchmaking.Lobby{Games: []*matchmaking.Game{}, GameID: 1},
		testSessionCookieConfig(),
		ownerUserID,
	)
}

var testUsernameCounter atomic.Int64

// testUsername returns a short, unique-per-process username (signup enforces
// a 32-char max, so this can't just be the test name + a timestamp) and
// registers cleanup that removes the user and any sessions referencing it -
// the FK has no ON DELETE CASCADE, so sessions must go first.
func testUsername(t *testing.T, db *sql.DB) string {
	t.Helper()
	n := testUsernameCounter.Add(1)
	username := fmt.Sprintf("ht%d%d", time.Now().Unix()%100000, n)

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM sessions WHERE user_id = (SELECT id FROM users WHERE username = ?)`, username)
		_, _ = db.Exec(`DELETE FROM users WHERE username = ?`, username)
	})

	return username
}
