package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"example.com/mishis4x/email"
	"example.com/mishis4x/persist"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

var testPasswordResetUserCounter int

// createTestUserWithRealEmail is createTestUser, but with a real email
// address on file - unlike createTestUser's plain account, a
// "forgot password" flow has nothing to find without one (see
// persist.GetUserByEmail's own doc comment on email_address being
// nullable).
func createTestUserWithRealEmail(t *testing.T, db *sql.DB, username, password, emailAddress string) int {
	t.Helper()

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)

	p := &persist.Persist{DB: db}
	id, err := p.CreateUser(t.Context(), persist.User{
		Username:     username,
		Password:     string(hashed),
		Status:       "active",
		EmailAddress: &emailAddress,
	})
	require.NoError(t, err)

	return id
}

func testPasswordResetEmail(t *testing.T) string {
	t.Helper()
	testPasswordResetUserCounter++
	return fmt.Sprintf("pwreset-test-%d-%d@example.com", os.Getpid(), testPasswordResetUserCounter)
}

// capturedEmail/fakeResendServerCapturing are shared with invites_test.go
// (the admin-notification-email feature needed the exact same "capture
// every request, not just succeed blindly" fake server) - defined there,
// reused here rather than duplicated.

// newTestServerWithPasswordReset builds a server with EmailService/
// AppBaseURL configured (adminUserID 0 - password reset doesn't care who,
// if anyone, is admin) via the fake-server-backed emailSvc passed in.
func newTestServerWithPasswordReset(t *testing.T, db *sql.DB, emailSvc *email.Service) (*httptest.Server, *http.Client) {
	t.Helper()

	d := newTestDataWithAdmin(db, 0, emailSvc, "https://mishis4x.com", "")
	ts := httptest.NewServer(d.NewRouter())
	t.Cleanup(ts.Close)

	return ts, newClient(t)
}

func TestRequestPasswordReset_Success(t *testing.T) {
	db := testDB(t)
	fake, received := fakeResendServerCapturing(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	ts, client := newTestServerWithPasswordReset(t, db, emailSvc)

	username := testUsername(t, db)
	emailAddress := testPasswordResetEmail(t)
	createTestUserWithRealEmail(t, db, username, "correctpass123", emailAddress)

	res := postJSON(t, client, ts.URL+"/api/user/password-reset/request", map[string]string{
		"email_address": emailAddress,
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	require.Len(t, *received, 1)
	notification := (*received)[0]
	require.Equal(t, []string{emailAddress}, notification.To)
	require.Contains(t, notification.Subject, "Reset")
	require.Contains(t, notification.HTML, "https://mishis4x.com/reset-password?token=")
}

func TestRequestPasswordReset_UnknownEmailStillReturns200(t *testing.T) {
	db := testDB(t)
	fake, received := fakeResendServerCapturing(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	ts, client := newTestServerWithPasswordReset(t, db, emailSvc)

	res := postJSON(t, client, ts.URL+"/api/user/password-reset/request", map[string]string{
		"email_address": testPasswordResetEmail(t),
	})
	require.Equal(t, http.StatusOK, res.StatusCode, "must not reveal whether the email matches an account")
	require.Empty(t, *received, "no account, no email")
}

func TestRequestPasswordReset_InvalidEmailFormat(t *testing.T) {
	db := testDB(t)
	fake, _ := fakeResendServerCapturing(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	ts, client := newTestServerWithPasswordReset(t, db, emailSvc)

	res := postJSON(t, client, ts.URL+"/api/user/password-reset/request", map[string]string{
		"email_address": "not-an-email",
	})
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestRequestPasswordReset_NotConfigured(t *testing.T) {
	db := testDB(t)
	// Plain newTestServer has no EmailService/AppBaseURL at all.
	ts, client := newTestServer(t, db)

	res := postJSON(t, client, ts.URL+"/api/user/password-reset/request", map[string]string{
		"email_address": testPasswordResetEmail(t),
	})
	require.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
}

func TestRequestPasswordReset_RateLimiting(t *testing.T) {
	db := testDB(t)
	fake, _ := fakeResendServerCapturing(t)
	emailSvc := email.NewServiceWithURL("test-key", "invites@mishis4x.com", fake.URL)
	ts, client := newTestServerWithPasswordReset(t, db, emailSvc)
	emailAddress := testPasswordResetEmail(t)

	var lastStatus int
	for i := 0; i < maxFailedAttempts; i++ {
		res := postJSON(t, client, ts.URL+"/api/user/password-reset/request", map[string]string{
			"email_address": emailAddress,
		})
		lastStatus = res.StatusCode
	}
	require.Equal(t, http.StatusOK, lastStatus)

	locked := postJSON(t, client, ts.URL+"/api/user/password-reset/request", map[string]string{
		"email_address": emailAddress,
	})
	require.Equal(t, http.StatusTooManyRequests, locked.StatusCode)
}

func TestConfirmPasswordReset_Success(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithPasswordReset(t, db, nil)
	p := &persist.Persist{DB: db}

	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "oldpassword123")

	// A logged-in session from before the reset - proving DeleteAllSessions
	// actually runs, not just that the password itself changed.
	loginClient := newClient(t)
	res := postJSON(t, loginClient, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "oldpassword123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	token, err := p.CreatePasswordReset(t.Context(), userID)
	require.NoError(t, err)

	confirmClient := newClient(t)
	confirmRes := postJSON(t, confirmClient, ts.URL+"/api/user/password-reset/confirm", map[string]string{
		"token":        token,
		"new_password": "brandnewpass456",
	})
	require.Equal(t, http.StatusOK, confirmRes.StatusCode)

	// Auto-login: the client that just confirmed the reset should already
	// be authenticated, no separate login step needed.
	dataRes, err := confirmClient.Get(ts.URL + "/api/data")
	require.NoError(t, err)
	defer func() { _ = dataRes.Body.Close() }()
	require.Equal(t, http.StatusOK, dataRes.StatusCode, "confirming a reset should log the caller straight in")

	// The pre-reset session must no longer work.
	oldSessionRes, err := loginClient.Get(ts.URL + "/api/data")
	require.NoError(t, err)
	defer func() { _ = oldSessionRes.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, oldSessionRes.StatusCode, "sessions from before the reset must be revoked")

	// The new password actually works; the old one no longer does.
	newLoginRes := postJSON(t, newClient(t), ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "brandnewpass456",
	})
	require.Equal(t, http.StatusOK, newLoginRes.StatusCode)

	oldLoginRes := postJSON(t, newClient(t), ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "oldpassword123",
	})
	require.Equal(t, http.StatusUnauthorized, oldLoginRes.StatusCode)
}

func TestConfirmPasswordReset_InvalidToken(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithPasswordReset(t, db, nil)

	res := postJSON(t, client, ts.URL+"/api/user/password-reset/confirm", map[string]string{
		"token":        "this-token-was-never-created",
		"new_password": "brandnewpass456",
	})
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestConfirmPasswordReset_WeakPassword(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithPasswordReset(t, db, nil)
	p := &persist.Persist{DB: db}

	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "oldpassword123")
	token, err := p.CreatePasswordReset(t.Context(), userID)
	require.NoError(t, err)

	res := postJSON(t, client, ts.URL+"/api/user/password-reset/confirm", map[string]string{
		"token":        token,
		"new_password": "short",
	})
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	// The token must still be usable - a rejected weak password shouldn't
	// burn the (single-use) reset link.
	res2 := postJSON(t, client, ts.URL+"/api/user/password-reset/confirm", map[string]string{
		"token":        token,
		"new_password": "actuallylongenough1",
	})
	require.Equal(t, http.StatusOK, res2.StatusCode)
}

func TestConfirmPasswordReset_TokenNotReusable(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithPasswordReset(t, db, nil)
	p := &persist.Persist{DB: db}

	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "oldpassword123")
	token, err := p.CreatePasswordReset(t.Context(), userID)
	require.NoError(t, err)

	res := postJSON(t, client, ts.URL+"/api/user/password-reset/confirm", map[string]string{
		"token":        token,
		"new_password": "brandnewpass456",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	res2 := postJSON(t, client, ts.URL+"/api/user/password-reset/confirm", map[string]string{
		"token":        token,
		"new_password": "someotherpass789",
	})
	require.Equal(t, http.StatusBadRequest, res2.StatusCode)
}
