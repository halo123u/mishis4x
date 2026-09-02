package ebay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// tokenRefreshBuffer requests a fresh token a bit before the cached one
// actually expires, so a request in flight never races an eBay-side
// expiry that lands mid-call.
const tokenRefreshBuffer = 2 * time.Minute

// oauthScope is the one scope the Browse API's public search actually
// needs under the Client Credentials Grant - see
// [[ebay-api-license-terms]] for why nothing broader (no user sign-in
// scopes) is required for this.
const oauthScope = "https://api.ebay.com/oauth/api_scope"

// tokenManager fetches and caches an app-level OAuth2 access token via the
// Client Credentials Grant (App ID + Cert ID, no end-user login involved -
// see [[ebay-api-license-terms]]), refreshing it once it's close to
// expiring rather than on every single call. In-memory, single-instance,
// same tradeoff as the rest of this package (see listingsCache's doc
// comment) - a restart just means the next call re-requests a token,
// which is cheap and unrate-limited at this volume.
type tokenManager struct {
	appID    string
	certID   string
	tokenURL string
	client   *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

func newTokenManager(appID, certID, tokenURL string, client *http.Client) *tokenManager {
	return &tokenManager{
		appID:    appID,
		certID:   certID,
		tokenURL: tokenURL,
		client:   client,
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// getToken returns a currently-valid access token, fetching a new one if
// none is cached or the cached one is within tokenRefreshBuffer of
// expiring.
func (m *tokenManager) getToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.accessToken != "" && time.Now().Before(m.expiresAt.Add(-tokenRefreshBuffer)) {
		return m.accessToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("scope", oauthScope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("building oauth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(m.appID, m.certID)

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting oauth token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// eBay's error responses here are almost always a JSON body like
		// {"error":"invalid_client","error_description":"..."} - genuinely
		// necessary for debugging an auth failure (which credential-level
		// problem, not just "some 4xx happened"), so surface it rather
		// than just the bare status code.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("unexpected status %d requesting oauth token from %s: %s", resp.StatusCode, m.tokenURL, body)
	}

	var parsed tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decoding oauth token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("oauth token response had no access_token")
	}

	m.accessToken = parsed.AccessToken
	m.expiresAt = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)

	return m.accessToken, nil
}
