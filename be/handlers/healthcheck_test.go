package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"example.com/mishis4x/persist"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthcheck_NoDBConfigured(t *testing.T) {
	d := &Data{}

	req := httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
	rec := httptest.NewRecorder()

	d.Healthcheck(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// This one's a real integration test against MySQL (see CLAUDE.md's testing
// philosophy) - a fake/mock DB can't tell us whether PingContext actually
// exercises a real connection the way the real healthcheck endpoint needs
// to. Skips (not fails) if no test DB is reachable.
func TestHealthcheck_DBReachable(t *testing.T) {
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

	d := &Data{P: persist.Persist{DB: db}}

	req := httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
	rec := httptest.NewRecorder()

	d.Healthcheck(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
