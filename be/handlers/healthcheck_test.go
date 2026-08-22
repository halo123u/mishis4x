package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/mishis4x/persist"
	"github.com/stretchr/testify/assert"
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
	db := testDB(t)
	d := &Data{P: persist.Persist{DB: db}}

	req := httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
	rec := httptest.NewRecorder()

	d.Healthcheck(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
