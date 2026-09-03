package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"testing"

	"example.com/mishis4x/persist"
	"github.com/stretchr/testify/require"
)

// These are integration tests against a real MySQL instance and a real
// httptest.Server running the app's actual router - see users_test.go's
// own doc comment for why. Skip (not fail) if no test DB is reachable.

var testInviteEmailCounter int

func testInviteEmail(t *testing.T, db *sql.DB) string {
	t.Helper()
	testInviteEmailCounter++
	email := fmt.Sprintf("ht-request-%d-%d@example.com", os.Getpid(), testInviteEmailCounter)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM invites WHERE email_address = ?`, email) })
	return email
}

func TestRequestInvite_Success(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	email := testInviteEmail(t, db)

	res := postJSON(t, client, ts.URL+"/api/invites/request", map[string]string{
		"email_address": email,
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)

	p := &persist.Persist{DB: db}
	requests, err := p.ListRequestedInvites(t.Context())
	require.NoError(t, err)

	found := false
	for _, r := range requests {
		if r.EmailAddress == email {
			found = true
			require.Equal(t, persist.InviteStatusRequested, r.Status)
		}
	}
	require.True(t, found, "request should show up in ListRequestedInvites")
}

func TestRequestInvite_InvalidEmail(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	res := postJSON(t, client, ts.URL+"/api/invites/request", map[string]string{
		"email_address": "not-an-email",
	})
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	require.Equal(t, "Please enter a valid email address.", decodeError(t, res))
}

func TestRequestInvite_MissingEmail(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	res := postJSON(t, client, ts.URL+"/api/invites/request", map[string]string{})
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	require.Equal(t, "Email address is required.", decodeError(t, res))
}

func TestRequestInvite_DuplicateStillReturnsGenericSuccess(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	email := testInviteEmail(t, db)

	res := postJSON(t, client, ts.URL+"/api/invites/request", map[string]string{
		"email_address": email,
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)

	// Same address again - not confirming/denying whether it already has
	// a pending request, so this must still look like a normal success,
	// not an error that would leak that information.
	res2 := postJSON(t, client, ts.URL+"/api/invites/request", map[string]string{
		"email_address": email,
	})
	require.Equal(t, http.StatusCreated, res2.StatusCode)

	p := &persist.Persist{DB: db}
	requests, err := p.ListRequestedInvites(t.Context())
	require.NoError(t, err)

	count := 0
	for _, r := range requests {
		if r.EmailAddress == email {
			count++
		}
	}
	require.Equal(t, 1, count, "a duplicate submission must not mint a second code")
}

func TestRequestInvite_RateLimiting(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	email := testInviteEmail(t, db)

	var lastStatus int
	for i := 0; i < maxFailedAttempts; i++ {
		res := postJSON(t, client, ts.URL+"/api/invites/request", map[string]string{
			"email_address": email,
		})
		lastStatus = res.StatusCode
	}
	require.Equal(t, http.StatusCreated, lastStatus, "the threshold-th attempt is still a normal response")

	lockedRes := postJSON(t, client, ts.URL+"/api/invites/request", map[string]string{
		"email_address": email,
	})
	require.Equal(t, http.StatusTooManyRequests, lockedRes.StatusCode)
}
