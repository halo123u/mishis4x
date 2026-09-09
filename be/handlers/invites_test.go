package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"example.com/mishis4x/email"
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

// TestValidateEmailAddress is a plain unit test (no DB, no server) - the
// HTTP-level tests below cover the same behavior end to end, but this is
// the fast, exhaustive place to enumerate edge cases like the missing-TLD
// one that prompted the dot-in-domain check in the first place.
func TestValidateEmailAddress(t *testing.T) {
	cases := []struct {
		name  string
		email string
		valid bool
	}{
		{"valid", "someone@example.com", true},
		{"valid with subdomain", "someone@mail.example.com", true},
		{"valid short TLD", "someone@example.io", true},
		{"empty", "", false},
		{"missing @", "not-an-email", false},
		{"display name syntax rejected", "Someone <someone@example.com>", false},
		{"missing TLD entirely", "oswaldo.almazo@gmail", false},
		{"trailing dot, nothing after", "someone@example.", false},
		{"single-char TLD", "someone@example.c", false},
		{"too long", strings.Repeat("a", maxEmailLen) + "@example.com", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := validateEmailAddress(c.email)
			if c.valid {
				require.Empty(t, msg, "expected %q to be accepted", c.email)
			} else {
				require.NotEmpty(t, msg, "expected %q to be rejected", c.email)
			}
		})
	}
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

// TestRequestInvite_EmailMissingTLD covers the exact gap found live: a
// domain with no dot at all (e.g. a typo'd "gmail" instead of
// "gmail.com") is syntactically valid RFC 5322 - net/mail.ParseAddress
// accepts it with no error - so validateEmailAddress needs its own
// explicit dot-in-domain check on top of that, not just delegate to
// ParseAddress.
func TestRequestInvite_EmailMissingTLD(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	res := postJSON(t, client, ts.URL+"/api/invites/request", map[string]string{
		"email_address": "oswaldo.almazo@gmail",
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

// capturedEmail mirrors just the JSON fields of email package's own
// unexported sendRequest that these tests actually need to assert on -
// can't decode into that type directly from outside the package, but the
// wire shape (what actually left this process) is what matters here
// anyway, not the internal Go type.
type capturedEmail struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

// fakeResendServerCapturing is fakeResendServerForAdmin (admin_test.go),
// but keeping every request it receives instead of just succeeding
// blindly - these tests care about content/count, not just "did the
// request-invite endpoint still 201."
func fakeResendServerCapturing(t *testing.T) (*httptest.Server, *[]capturedEmail) {
	t.Helper()
	var mu sync.Mutex
	var received []capturedEmail

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body capturedEmail
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		received = append(received, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"fake-id"}`))
	}))
	t.Cleanup(server.Close)
	return server, &received
}

func TestRequestInvite_NotifiesAdmin(t *testing.T) {
	db := testDB(t)
	fake, received := fakeResendServerCapturing(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	d := newTestDataWithAdminNotification(db, emailSvc, "https://mishis4x.com", "owner@example.com")
	ts := httptest.NewServer(d.NewRouter())
	t.Cleanup(ts.Close)
	client := newClient(t)

	requesterEmail := testInviteEmail(t, db)
	res := postJSON(t, client, ts.URL+"/api/invites/request", map[string]string{
		"email_address": requesterEmail,
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)

	require.Len(t, *received, 1, "exactly one notification email must go out for one new request")
	notification := (*received)[0]
	require.Equal(t, []string{"owner@example.com"}, notification.To)
	require.Contains(t, notification.Subject, "invite request")
	require.Contains(t, notification.HTML, "https://mishis4x.com/admin", "must link straight to the approve page")
	require.Contains(t, notification.HTML, requesterEmail, "must say who's asking")
}

func TestRequestInvite_DuplicateDoesNotReNotify(t *testing.T) {
	db := testDB(t)
	fake, received := fakeResendServerCapturing(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	d := newTestDataWithAdminNotification(db, emailSvc, "https://mishis4x.com", "owner@example.com")
	ts := httptest.NewServer(d.NewRouter())
	t.Cleanup(ts.Close)
	client := newClient(t)

	requesterEmail := testInviteEmail(t, db)
	for range 2 {
		res := postJSON(t, client, ts.URL+"/api/invites/request", map[string]string{
			"email_address": requesterEmail,
		})
		require.Equal(t, http.StatusCreated, res.StatusCode)
	}

	require.Len(t, *received, 1, "resubmitting an already-pending request must not re-notify")
}

// TestRequestInvite_NotConfiguredSkipsSilently covers the default,
// unconfigured case (newTestServer's plain Data has no EmailService/
// AppBaseURL/AdminNotificationEmail at all) - the real point of this test
// is simply that RequestInvite still succeeds rather than erroring or
// panicking on a nil EmailService, since every existing
// TestRequestInvite_* test above already exercises that path implicitly;
// this one just says so explicitly.
func TestRequestInvite_NotConfiguredSkipsSilently(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	requesterEmail := testInviteEmail(t, db)

	res := postJSON(t, client, ts.URL+"/api/invites/request", map[string]string{
		"email_address": requesterEmail,
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)
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
