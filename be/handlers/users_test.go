package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"example.com/mishis4x/ebay"
	"example.com/mishis4x/matchmaking"
	"example.com/mishis4x/persist"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// These are integration tests against a real MySQL instance and a real
// httptest.Server running the app's actual router (middleware, routing,
// everything) - not calling handler funcs in isolation. See CLAUDE.md's
// testing philosophy. Skip (not fail) if no test DB is reachable.

func newTestServer(t *testing.T, db *sql.DB) (*httptest.Server, *http.Client) {
	t.Helper()

	d := newTestData(db)
	ts := httptest.NewServer(d.NewRouter())
	t.Cleanup(ts.Close)

	return ts, newClient(t)
}

// newTestServerWithEbay is newTestServer, but wired with a real (fake-
// server-backed) ebay.Service instead of nil - for ebay_listings_test.go.
func newTestServerWithEbay(t *testing.T, db *sql.DB, ebaySvc *ebay.Service) (*httptest.Server, *http.Client) {
	t.Helper()

	d := newTestDataWithEbay(db, ebaySvc)
	ts := httptest.NewServer(d.NewRouter())
	t.Cleanup(ts.Close)

	return ts, newClient(t)
}

// newTestServerWithEbayDisabled wires a real ebay.Service (so a test can
// prove the EbayListingsDisabled kill switch blocks the route even with
// working credentials, not just when Ebay is nil) alongside
// EbayListingsDisabled: true.
func newTestServerWithEbayDisabled(t *testing.T, db *sql.DB, ebaySvc *ebay.Service) (*httptest.Server, *http.Client) {
	t.Helper()

	d := NewData(
		persist.Persist{DB: db},
		&matchmaking.Lobby{Games: []*matchmaking.Game{}, GameID: 1},
		testSessionCookieConfig(),
		0,
		false,
		ebaySvc,
		true,
	)
	ts := httptest.NewServer(d.NewRouter())
	t.Cleanup(ts.Close)

	return ts, newClient(t)
}

// newClient builds an independent *http.Client with its own cookie jar.
// Deliberately NOT built from ts.Client() for a second/third client in the
// same test - httptest.Server.Client() caches and returns the SAME
// *http.Client on repeated calls, so two "independent" clients built that
// way would secretly alias one another's .Jar.
func newClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	return &http.Client{Jar: jar}
}

// createTestUser inserts a user directly (bypassing the signup endpoint)
// with a real bcrypt hash, for tests that only care about login/change-
// password behavior against an already-existing account.
func createTestUser(t *testing.T, db *sql.DB, username, password string) int {
	t.Helper()

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)

	p := &persist.Persist{DB: db}
	id, err := p.CreateUser(t.Context(), persist.User{
		Username: username,
		Password: string(hashed),
		Status:   "active",
	})
	require.NoError(t, err)

	return id
}

func postJSON(t *testing.T, client *http.Client, url string, body map[string]string) *http.Response {
	t.Helper()

	b, err := json.Marshal(body)
	require.NoError(t, err)

	res, err := client.Post(url, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })

	return res
}

func decodeError(t *testing.T, res *http.Response) string {
	t.Helper()

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	return body.Error
}

func TestUserCreate_Success(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	username := testUsername(t, db)

	res := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
		"username":    username,
		"password":    "validpass123",
		"invite_code": testApprovedInvite(t, db),
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)

	// The signup response should have set a working session cookie -
	// confirm it actually authenticates the follow-up request.
	dataRes, err := client.Get(ts.URL + "/api/data")
	require.NoError(t, err)
	defer func() { _ = dataRes.Body.Close() }()
	require.Equal(t, http.StatusOK, dataRes.StatusCode)

	var body struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	require.NoError(t, json.NewDecoder(dataRes.Body).Decode(&body))
	require.Equal(t, username, body.User.Username)
}

func TestUserCreate_DuplicateUsername(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	username := testUsername(t, db)

	res := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
		"username":    username,
		"password":    "validpass123",
		"invite_code": testApprovedInvite(t, db),
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)

	// Invites are single-use (see persist.RedeemInvite), so this second
	// attempt needs its own fresh, valid one to actually reach the
	// duplicate-username check rather than failing on an already-used
	// invite instead.
	res2 := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
		"username":    username,
		"password":    "anotherpass456",
		"invite_code": testApprovedInvite(t, db),
	})
	require.Equal(t, http.StatusConflict, res2.StatusCode)
	require.Equal(t, "That username is already taken.", decodeError(t, res2))
}

func TestUserCreate_NoInviteCode(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	username := testUsername(t, db)

	res := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
		"username": username,
		"password": "validpass123",
	})
	require.Equal(t, http.StatusForbidden, res.StatusCode)
	require.Equal(t, "This invite link is invalid or has already been used.", decodeError(t, res))
}

func TestUserCreate_InvalidInviteCode(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	username := testUsername(t, db)

	res := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
		"username":    username,
		"password":    "validpass123",
		"invite_code": "this-code-was-never-minted",
	})
	require.Equal(t, http.StatusForbidden, res.StatusCode)
	require.Equal(t, "This invite link is invalid or has already been used.", decodeError(t, res))
}

func TestUserCreate_AlreadyUsedInviteCode(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	invite := testApprovedInvite(t, db)

	res := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
		"username":    testUsername(t, db),
		"password":    "validpass123",
		"invite_code": invite,
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)

	// Same invite, a different (available) username - the invite itself
	// being spent is what should block this, not a username collision.
	res2 := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
		"username":    testUsername(t, db),
		"password":    "anotherpass456",
		"invite_code": invite,
	})
	require.Equal(t, http.StatusForbidden, res2.StatusCode)
	require.Equal(t, "This invite link is invalid or has already been used.", decodeError(t, res2))
}

// TestUserCreate_PasswordTooShort and TestUserCreate_MissingUsername
// deliberately don't pass an invite_code - input validation
// (validateSignupInput) runs before the invite is ever checked (see
// handlers.UserCreate), so these fail on the same 400 regardless.

func TestUserCreate_PasswordTooShort(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	username := testUsername(t, db)

	res := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
		"username": username,
		"password": "short",
	})
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	require.Equal(t, "Password must be at least 8 characters.", decodeError(t, res))
}

func TestUserCreate_MissingUsername(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	res := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
		"username": "",
		"password": "validpass123",
	})
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	require.Equal(t, "Username is required.", decodeError(t, res))
}

func TestUserLogin_Success(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	username := testUsername(t, db)
	createTestUser(t, db, username, "correctpass123")

	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	dataRes, err := client.Get(ts.URL + "/api/data")
	require.NoError(t, err)
	defer func() { _ = dataRes.Body.Close() }()
	require.Equal(t, http.StatusOK, dataRes.StatusCode)
}

func TestUserLogin_WrongPasswordAndNoSuchUser_SameError(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	username := testUsername(t, db)
	createTestUser(t, db, username, "correctpass123")

	wrongPassRes := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "wrongpassword",
	})
	require.Equal(t, http.StatusUnauthorized, wrongPassRes.StatusCode)
	wrongPassMsg := decodeError(t, wrongPassRes)

	noSuchUserRes := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": testUsername(t, db), // a different, never-created username
		"password": "whatever123",
	})
	require.Equal(t, http.StatusUnauthorized, noSuchUserRes.StatusCode)
	noSuchUserMsg := decodeError(t, noSuchUserRes)

	// The whole point: identical response either way, so this endpoint
	// can't be used to enumerate which usernames exist.
	require.Equal(t, wrongPassMsg, noSuchUserMsg)
	require.Equal(t, "Incorrect username or password.", wrongPassMsg)
}

func TestUserLogin_RateLimiting(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	username := testUsername(t, db)
	createTestUser(t, db, username, "correctpass123")

	var lastStatus int
	for i := 0; i < maxFailedAttempts; i++ {
		res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
			"username": username,
			"password": "wrongpassword",
		})
		lastStatus = res.StatusCode
	}
	require.Equal(t, http.StatusUnauthorized, lastStatus, "the threshold-th attempt is still just a normal failed login")

	lockedRes := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "wrongpassword",
	})
	require.Equal(t, http.StatusTooManyRequests, lockedRes.StatusCode)

	// Even the CORRECT password should be locked out now.
	correctButLockedRes := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusTooManyRequests, correctButLockedRes.StatusCode)
}

func TestUserCreate_RateLimiting(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	username := testUsername(t, db)
	createTestUser(t, db, username, "correctpass123")

	// Every attempt below hits the duplicate-username (409) branch, since
	// username already exists - repeatedly probing that is exactly the
	// enumeration angle this limiter exists to slow down. Invites are
	// single-use (see persist.RedeemInvite), so each attempt needs its own
	// fresh one to actually reach the duplicate-username check.
	var lastStatus int
	for i := 0; i < maxFailedAttempts; i++ {
		res := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
			"username":    username,
			"password":    "someotherpass123",
			"invite_code": testApprovedInvite(t, db),
		})
		lastStatus = res.StatusCode
	}
	require.Equal(t, http.StatusConflict, lastStatus, "the threshold-th attempt is still just a normal duplicate-username response")

	// No invite_code needed here - the rate-limit check now runs before
	// the invite is claimed (see handlers.UserCreate), specifically so a
	// lockout like this one doesn't need to burn one.
	lockedRes := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
		"username": username,
		"password": "someotherpass123",
	})
	require.Equal(t, http.StatusTooManyRequests, lockedRes.StatusCode)

	// A completely different, available username must be unaffected - the
	// limiter is keyed per-username, not global.
	freeUsername := testUsername(t, db)
	freeRes := postJSON(t, client, ts.URL+"/api/user/create", map[string]string{
		"username":    freeUsername,
		"password":    "someotherpass123",
		"invite_code": testApprovedInvite(t, db),
	})
	require.Equal(t, http.StatusCreated, freeRes.StatusCode)
}

func TestRequestBodyTooLarge(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	// Must be syntactically valid JSON, just oversized - a body that's
	// simply garbage bytes fails on JSON syntax after reading only the
	// first few bytes, never actually exercising the size limit.
	hugeValue := strings.Repeat("a", maxRequestBodyBytes+1)
	oversized := []byte(`{"username":"` + hugeValue + `","password":"whatever"}`)

	res, err := client.Post(ts.URL+"/api/user/login", "application/json", bytes.NewReader(oversized))
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()

	require.Equal(t, http.StatusRequestEntityTooLarge, res.StatusCode)
}

func TestRecoverMiddleware_TurnsPanicIntoLogged500(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	rec := httptest.NewRecorder()

	recoverMiddleware(panicking).ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "Something went wrong.", decodeError(t, rec.Result()))
}

func TestUserLogout_ActuallyRevokesSession(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	username := testUsername(t, db)
	createTestUser(t, db, username, "correctpass123")

	loginRes := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, loginRes.StatusCode)

	// Capture the raw token before logout clears the jar's copy of it, so we
	// can replay the EXACT pre-logout cookie afterward - proving server-side
	// revocation, not just that the client forgot its own cookie.
	jarURL, err := url.Parse(ts.URL)
	require.NoError(t, err)
	cookies := client.Jar.Cookies(jarURL)
	require.NotEmpty(t, cookies)
	rawToken := cookies[0].Value

	logoutReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/logout", nil)
	require.NoError(t, err)
	logoutRes, err := client.Do(logoutReq)
	require.NoError(t, err)
	defer func() { _ = logoutRes.Body.Close() }()
	require.Equal(t, http.StatusOK, logoutRes.StatusCode)

	// Replay the exact pre-logout token directly (bypassing the client's
	// jar, which has already updated to the cleared cookie).
	replayReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/data", nil)
	require.NoError(t, err)
	replayReq.AddCookie(&http.Cookie{Name: "session", Value: rawToken})

	// A bare client with no jar of its own - the only cookie on this
	// request is the exact one manually attached above.
	replayRes, err := (&http.Client{}).Do(replayReq)
	require.NoError(t, err)
	defer func() { _ = replayRes.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, replayRes.StatusCode, "the pre-logout token must be rejected - logout should delete it server-side, not just clear the client cookie")
}

func TestChangePassword_RevokesOtherSessionsButNotThisOne(t *testing.T) {
	db := testDB(t)
	ts, clientA := newTestServer(t, db)
	username := testUsername(t, db)
	createTestUser(t, db, username, "originalpass123")

	loginAndKeepCookies := func(client *http.Client) {
		res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
			"username": username,
			"password": "originalpass123",
		})
		require.Equal(t, http.StatusOK, res.StatusCode)
	}

	loginAndKeepCookies(clientA)

	clientB := newClient(t)
	loginAndKeepCookies(clientB)

	changeRes := postJSON(t, clientA, ts.URL+"/api/user/password", map[string]string{
		"currentPassword": "originalpass123",
		"newPassword":     "brandnewpass456",
	})
	require.Equal(t, http.StatusOK, changeRes.StatusCode)

	// Session A (the one that made the change) must still work.
	dataA, err := clientA.Get(ts.URL + "/api/data")
	require.NoError(t, err)
	defer func() { _ = dataA.Body.Close() }()
	require.Equal(t, http.StatusOK, dataA.StatusCode)

	// Session B (a different device) must now be revoked.
	dataB, err := clientB.Get(ts.URL + "/api/data")
	require.NoError(t, err)
	defer func() { _ = dataB.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, dataB.StatusCode)

	// Old password no longer works, new password does.
	oldPassRes := postJSON(t, clientB, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "originalpass123",
	})
	require.Equal(t, http.StatusUnauthorized, oldPassRes.StatusCode)

	newPassRes := postJSON(t, clientB, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "brandnewpass456",
	})
	require.Equal(t, http.StatusOK, newPassRes.StatusCode)
}

func TestChangePassword_WrongCurrentPassword(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)
	username := testUsername(t, db)
	createTestUser(t, db, username, "originalpass123")

	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "originalpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	changeRes := postJSON(t, client, ts.URL+"/api/user/password", map[string]string{
		"currentPassword": "notmycurrentpassword",
		"newPassword":     "brandnewpass456",
	})
	require.Equal(t, http.StatusUnauthorized, changeRes.StatusCode)
	require.Equal(t, "Current password is incorrect.", decodeError(t, changeRes))
}

func TestChangePassword_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	// No login first - fresh client, no session cookie at all.
	res := postJSON(t, client, ts.URL+"/api/user/password", map[string]string{
		"currentPassword": "whatever",
		"newPassword":     "brandnewpass456",
	})
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}
