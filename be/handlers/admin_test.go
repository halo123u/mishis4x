package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"example.com/mishis4x/email"
	"example.com/mishis4x/persist"
	"github.com/stretchr/testify/require"
)

// fakeResendServerForAdmin mirrors email package's own test fixture (and
// fakeEbayServer's same reasoning in ebay_listings_test.go) - kept as its
// own small copy here rather than exported from the email package purely
// for a test fixture. Always succeeds - admin_test.go isn't exercising
// Resend failure handling, ApproveInviteRequest_EmailSendFailure below is
// the one test that needs a different response.
func fakeResendServerForAdmin(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"fake-id"}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// newTestServerWithAdmin builds a server with adminUserID recognized as
// the admin (see AdminUserID's doc comment) and a fake-server-backed
// email.Service, then creates and logs in as exactly that user - the
// returned client is already authenticated as the admin, ready to hit
// /api/admin/... routes without a separate login step in every test.
func newTestServerWithAdmin(t *testing.T, db *sql.DB, emailSvc *email.Service) (*httptest.Server, *http.Client) {
	t.Helper()

	username := testUsername(t, db)
	adminUserID := createTestUser(t, db, username, "correctpass123")

	d := newTestDataWithAdmin(db, adminUserID, emailSvc, "https://mishis4x.com")
	ts := httptest.NewServer(d.NewRouter())
	t.Cleanup(ts.Close)

	client := newClient(t)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	return ts, client
}

// requestedInviteForAdmin creates a fresh 'requested' invite for
// emailAddress and returns its id - bypassing HTTP (matching
// createTestUser's own bypass-HTTP-for-setup convention) since these
// tests care about the admin routes acting on a request, not the
// request-submission endpoint itself (already covered by
// invites_test.go).
func requestedInviteForAdmin(t *testing.T, db *sql.DB, emailAddress string) int {
	t.Helper()
	p := &persist.Persist{DB: db}
	require.NoError(t, p.CreateInviteRequest(t.Context(), emailAddress))

	requests, err := p.ListRequestedInvites(t.Context())
	require.NoError(t, err)
	for _, r := range requests {
		if r.EmailAddress == emailAddress {
			return r.ID
		}
	}
	t.Fatalf("just-created request for %s not found in ListRequestedInvites", emailAddress)
	return 0
}

var testAdminEmailCounter int

func testAdminInviteEmail(t *testing.T) string {
	t.Helper()
	testAdminEmailCounter++
	return fmt.Sprintf("admin-test-%d-%d@example.com", os.Getpid(), testAdminEmailCounter)
}

func TestListPendingInvites_Success(t *testing.T) {
	db := testDB(t)
	fake := fakeResendServerForAdmin(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	ts, client := newTestServerWithAdmin(t, db, emailSvc)

	emailAddress := testAdminInviteEmail(t)
	requestedInviteForAdmin(t, db, emailAddress)

	res, err := client.Get(ts.URL + "/api/admin/invites")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var got []struct {
		ID           int    `json:"id"`
		EmailAddress string `json:"email_address"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))

	found := false
	for _, r := range got {
		if r.EmailAddress == emailAddress {
			found = true
		}
	}
	require.True(t, found, "the just-created request should show up in the admin list")
}

func TestListPendingInvites_NonAdminForbidden(t *testing.T) {
	db := testDB(t)
	// adminUserID (created inside newTestServerWithAdmin) is a different
	// account than the one logging in here - this second client is a
	// normal, unrelated authenticated user.
	fake := fakeResendServerForAdmin(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	ts, _ := newTestServerWithAdmin(t, db, emailSvc)

	username := testUsername(t, db)
	createTestUser(t, db, username, "correctpass123")
	client := newClient(t)
	loginRes := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, loginRes.StatusCode)

	res, err := client.Get(ts.URL + "/api/admin/invites")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestListPendingInvites_Unauthenticated(t *testing.T) {
	db := testDB(t)
	fake := fakeResendServerForAdmin(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	ts, _ := newTestServerWithAdmin(t, db, emailSvc)

	res, err := http.Get(ts.URL + "/api/admin/invites")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestApproveInviteRequest_Success(t *testing.T) {
	db := testDB(t)
	fake := fakeResendServerForAdmin(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	ts, client := newTestServerWithAdmin(t, db, emailSvc)

	emailAddress := testAdminInviteEmail(t)
	id := requestedInviteForAdmin(t, db, emailAddress)

	res := postJSON(t, client, fmt.Sprintf("%s/api/admin/invites/%d/approve", ts.URL, id), nil)
	require.Equal(t, http.StatusOK, res.StatusCode)

	var status string
	require.NoError(t, db.QueryRow("SELECT status FROM invites WHERE id = ?", id).Scan(&status))
	require.Equal(t, persist.InviteStatusApproved, status)
}

func TestApproveInviteRequest_AlreadyDecided(t *testing.T) {
	db := testDB(t)
	fake := fakeResendServerForAdmin(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	ts, client := newTestServerWithAdmin(t, db, emailSvc)

	emailAddress := testAdminInviteEmail(t)
	id := requestedInviteForAdmin(t, db, emailAddress)

	res := postJSON(t, client, fmt.Sprintf("%s/api/admin/invites/%d/approve", ts.URL, id), nil)
	require.Equal(t, http.StatusOK, res.StatusCode)

	res2 := postJSON(t, client, fmt.Sprintf("%s/api/admin/invites/%d/approve", ts.URL, id), nil)
	require.Equal(t, http.StatusConflict, res2.StatusCode)
}

func TestApproveInviteRequest_EmailNotConfigured(t *testing.T) {
	db := testDB(t)
	// nil email.Service, matching how the server boots when
	// RESEND_API_KEY isn't set (see loadEmailServiceForServer).
	ts, client := newTestServerWithAdmin(t, db, nil)

	emailAddress := testAdminInviteEmail(t)
	id := requestedInviteForAdmin(t, db, emailAddress)

	res := postJSON(t, client, fmt.Sprintf("%s/api/admin/invites/%d/approve", ts.URL, id), nil)
	require.Equal(t, http.StatusServiceUnavailable, res.StatusCode)

	// Not approved - failing before touching the DB means a fixed config
	// (or the CLI instead) can still act on this same request.
	var status string
	require.NoError(t, db.QueryRow("SELECT status FROM invites WHERE id = ?", id).Scan(&status))
	require.Equal(t, persist.InviteStatusRequested, status)
}

func TestDenyInviteRequest_Success(t *testing.T) {
	db := testDB(t)
	fake := fakeResendServerForAdmin(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	ts, client := newTestServerWithAdmin(t, db, emailSvc)

	emailAddress := testAdminInviteEmail(t)
	id := requestedInviteForAdmin(t, db, emailAddress)

	res := postJSON(t, client, fmt.Sprintf("%s/api/admin/invites/%d/deny", ts.URL, id), nil)
	require.Equal(t, http.StatusOK, res.StatusCode)

	var status string
	require.NoError(t, db.QueryRow("SELECT status FROM invites WHERE id = ?", id).Scan(&status))
	require.Equal(t, persist.InviteStatusDenied, status)
}

func TestDenyInviteRequest_NonAdminForbidden(t *testing.T) {
	db := testDB(t)
	fake := fakeResendServerForAdmin(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	ts, _ := newTestServerWithAdmin(t, db, emailSvc)

	emailAddress := testAdminInviteEmail(t)
	id := requestedInviteForAdmin(t, db, emailAddress)

	username := testUsername(t, db)
	createTestUser(t, db, username, "correctpass123")
	client := newClient(t)
	loginRes := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, loginRes.StatusCode)

	res := postJSON(t, client, fmt.Sprintf("%s/api/admin/invites/%d/deny", ts.URL, id), nil)
	require.Equal(t, http.StatusForbidden, res.StatusCode)

	// Still requested - the forbidden attempt must not have side effects.
	var status string
	require.NoError(t, db.QueryRow("SELECT status FROM invites WHERE id = ?", id).Scan(&status))
	require.Equal(t, persist.InviteStatusRequested, status)
}
