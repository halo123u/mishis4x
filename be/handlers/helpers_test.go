package handlers

import (
	"database/sql"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"example.com/mishis4x/ebay"
	"example.com/mishis4x/email"
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
// for tests to spin up a router from. CollectionOwnerUserID/
// CollectionAllowAllUsers are unset - not relevant here since no route
// currently checks them (see CollectionOwnerUserID's doc comment); a test
// exercising canAccessCollection/ownerOnlyMiddleware directly constructs its
// own Data{...} instead (see owner_only_test.go).
func newTestData(db *sql.DB) *Data {
	return newTestDataWithEbay(db, nil)
}

// newTestDataWithEbay is newTestData, but with an explicit ebay.Service -
// for the one test file (ebay_listings_test.go) that needs a real (fake-
// server-backed) one instead of the nil every other test gets.
func newTestDataWithEbay(db *sql.DB, ebaySvc *ebay.Service) *Data {
	return NewData(
		persist.Persist{DB: db},
		&matchmaking.Lobby{Games: []*matchmaking.Game{}, GameID: 1},
		testSessionCookieConfig(),
		0,
		false,
		ebaySvc,
		false,
		false,
		0,
		nil,
		"",
	)
}

// newTestDataWithPriceTrends is newTestData, but with PriceTrendsEnabled
// true - for price_trends_test.go, which needs the feature actually on
// to test its real behavior (every other test gets the off-by-default
// state, matching production until this is explicitly turned on).
func newTestDataWithPriceTrends(db *sql.DB) *Data {
	return NewData(
		persist.Persist{DB: db},
		&matchmaking.Lobby{Games: []*matchmaking.Game{}, GameID: 1},
		testSessionCookieConfig(),
		0,
		false,
		nil,
		false,
		true,
		0,
		nil,
		"",
	)
}

// newTestDataWithAdmin is newTestData, but with adminUserID recognized as
// the admin and a real (fake-server-backed) email.Service - for
// admin_test.go's tests, which need both to exercise the actual
// approve-and-email flow rather than just the 403-if-not-admin gate.
func newTestDataWithAdmin(db *sql.DB, adminUserID int, emailSvc *email.Service, appBaseURL string) *Data {
	return NewData(
		persist.Persist{DB: db},
		&matchmaking.Lobby{Games: []*matchmaking.Game{}, GameID: 1},
		testSessionCookieConfig(),
		0,
		false,
		nil,
		false,
		false,
		adminUserID,
		emailSvc,
		appBaseURL,
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

var testInviteCounter atomic.Int64

// testApprovedInvite inserts an invites row that's already 'approved',
// straight via SQL (bypassing the real request -> invite-approve flow
// and its email send entirely, matching how createTestUser bypasses HTTP
// for direct user creation) - signup requires a redeemable code (see
// handlers.UserCreate), and since each one is single-use, any test
// driving /api/user/create through the invite-redemption path needs its
// own call to this per attempt.
func testApprovedInvite(t *testing.T, db *sql.DB) string {
	t.Helper()

	code, err := persist.NewInviteCode()
	require.NoError(t, err)

	n := testInviteCounter.Add(1)
	email := fmt.Sprintf("ht-invite-%d-%d@example.com", time.Now().Unix()%100000, n)

	_, err = db.Exec(
		`INSERT INTO invites (code, status, email_address) VALUES (?, 'approved', ?)`,
		code, email,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM invites WHERE code = ?`, code)
	})

	return code
}
