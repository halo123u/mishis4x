package ebay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func tokenServer(t *testing.T, expiresIn int, requestCount *int32) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(requestCount, 1)

		user, pass, ok := r.BasicAuth()
		require.True(t, ok, "must use HTTP Basic Auth")
		require.Equal(t, "app-id", user)
		require.Equal(t, "cert-id", pass)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "client_credentials", r.PostForm.Get("grant_type"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token-` + strconv.Itoa(int(n)) + `","expires_in":` + strconv.Itoa(expiresIn) + `}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestTokenManager_CachesUntilExpiry(t *testing.T) {
	var requests int32
	server := tokenServer(t, 7200, &requests)

	m := newTokenManager("app-id", "cert-id", server.URL, &http.Client{})

	tok1, err := m.getToken(t.Context())
	require.NoError(t, err)
	require.Equal(t, "token-1", tok1)

	tok2, err := m.getToken(t.Context())
	require.NoError(t, err)
	require.Equal(t, tok1, tok2, "a still-valid token must be reused, not re-requested")
	require.Equal(t, int32(1), atomic.LoadInt32(&requests), "only one real request should have been made")
}

func TestTokenManager_RefreshesWithinBuffer(t *testing.T) {
	var requests int32
	// expires_in shorter than tokenRefreshBuffer - the cached token should
	// be treated as already-expiring on the very next call.
	server := tokenServer(t, 30, &requests)

	m := newTokenManager("app-id", "cert-id", server.URL, &http.Client{})

	tok1, err := m.getToken(t.Context())
	require.NoError(t, err)

	tok2, err := m.getToken(t.Context())
	require.NoError(t, err)
	require.NotEqual(t, tok1, tok2, "a token expiring within tokenRefreshBuffer must be refreshed, not reused")
	require.Equal(t, int32(2), atomic.LoadInt32(&requests))
}

func TestTokenManager_NonOKStatusIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	m := newTokenManager("app-id", "cert-id", server.URL, &http.Client{})

	_, err := m.getToken(t.Context())
	require.Error(t, err)
}

func TestTokenManager_RespectsContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":7200}`))
	}))
	t.Cleanup(server.Close)

	m := newTokenManager("app-id", "cert-id", server.URL, &http.Client{})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
	defer cancel()

	_, err := m.getToken(ctx)
	require.Error(t, err)
}
