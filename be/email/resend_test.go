package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeResendServer stands in for Resend's real API - real HTTP round trip
// against a fake server, not a mocked client, matching CLAUDE.md's testing
// philosophy for this codebase's other external integrations (see
// be/ebay's own fake-server-backed tests).
func fakeResendServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func TestSend_Success(t *testing.T) {
	var gotAuth string
	var gotBody sendRequest

	ts := fakeResendServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"fake-id"}`))
	})

	s := NewService("test-key", "invites@mishis4x.com")
	s.client = ts.Client()
	s.apiURL = ts.URL
	err := s.Send(t.Context(), "someone@example.com", "subject", "<p>hi</p>")
	require.NoError(t, err)

	require.Equal(t, "Bearer test-key", gotAuth)
	require.Equal(t, "invites@mishis4x.com", gotBody.From)
	require.Equal(t, []string{"someone@example.com"}, gotBody.To)
	require.Equal(t, "subject", gotBody.Subject)
	require.Equal(t, "<p>hi</p>", gotBody.HTML)
}

func TestSend_ErrorStatus(t *testing.T) {
	ts := fakeResendServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	})

	s := NewService("bad-key", "invites@mishis4x.com")
	s.client = ts.Client()
	s.apiURL = ts.URL
	err := s.Send(t.Context(), "someone@example.com", "subject", "<p>hi</p>")
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
}

func TestSendInviteEmail_IncludesLink(t *testing.T) {
	var gotBody sendRequest

	ts := fakeResendServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusOK)
	})

	s := NewService("test-key", "invites@mishis4x.com")
	s.client = ts.Client()
	s.apiURL = ts.URL
	err := s.SendInviteEmail(t.Context(), "someone@example.com", "https://mishis4x.com/sign-up?invite=abc123")
	require.NoError(t, err)
	require.Contains(t, gotBody.HTML, "https://mishis4x.com/sign-up?invite=abc123")
}

func TestSendInviteRequestNotification_IncludesLinkAndEscapesRequesterEmail(t *testing.T) {
	var gotBody sendRequest

	ts := fakeResendServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusOK)
	})

	s := NewService("test-key", "invites@mishis4x.com")
	s.client = ts.Client()
	s.apiURL = ts.URL
	// "&" is a valid unquoted RFC 5322 local-part character - a real
	// address this could actually see, and exactly the kind of thing
	// that needs escaping (to "&amp;") rather than dropped straight into
	// the HTML body as-is.
	err := s.SendInviteRequestNotification(t.Context(), "owner@example.com", "foo&bar@example.com", "https://mishis4x.com/admin")
	require.NoError(t, err)

	require.Equal(t, []string{"owner@example.com"}, gotBody.To)
	require.Contains(t, gotBody.HTML, "https://mishis4x.com/admin")
	require.Contains(t, gotBody.HTML, "foo&amp;bar@example.com")
	require.NotContains(t, gotBody.HTML, "foo&bar@example.com")
}

func TestSendPasswordResetEmail_IncludesLink(t *testing.T) {
	var gotBody sendRequest

	ts := fakeResendServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusOK)
	})

	s := NewService("test-key", "invites@mishis4x.com")
	s.client = ts.Client()
	s.apiURL = ts.URL
	err := s.SendPasswordResetEmail(t.Context(), "someone@example.com", "https://mishis4x.com/reset-password?token=abc123")
	require.NoError(t, err)

	require.Equal(t, []string{"someone@example.com"}, gotBody.To)
	require.Contains(t, gotBody.Subject, "Reset")
	require.Contains(t, gotBody.HTML, "https://mishis4x.com/reset-password?token=abc123")
}
