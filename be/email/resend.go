// Package email sends transactional email via Resend's HTTP API
// (https://resend.com/docs/api-reference/emails/send-email) - a minimal
// hand-rolled client rather than pulling in Resend's official SDK,
// matching be/ebay's same "small REST surface, no heavy dependency"
// approach for a single-endpoint integration.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultResendAPIURL = "https://api.resend.com/emails"
	requestTimeout      = 10 * time.Second
)

// Service sends email from a fixed, pre-verified sender address (see
// NewService - Resend requires the sending domain to be verified via DNS
// before it'll relay anything from it).
type Service struct {
	apiKey string
	from   string
	// apiURL is a field (not a package const referenced directly in
	// Send) specifically so tests can point it at a fake server -
	// real HTTP round trip against a fake server, not a mocked client,
	// matching this codebase's testing philosophy elsewhere.
	apiURL string
	client *http.Client
}

// NewService builds a Service. from must be an address on a domain
// already verified with Resend (SPF/DKIM records added at the
// registrar) - sends from an unverified domain are rejected by Resend
// itself, not something this package can detect ahead of time.
func NewService(apiKey, from string) *Service {
	return NewServiceWithURL(apiKey, from, defaultResendAPIURL)
}

// NewServiceWithURL is the same construction NewService does, but with
// an explicit API URL - exported for tests (in this package and
// be/handlers) to point at a local httptest.Server instead of Resend's
// real API, same convention be/ebay's NewServiceWithURLs uses. Not meant
// for production use - call NewService there instead.
func NewServiceWithURL(apiKey, from, apiURL string) *Service {
	return &Service{
		apiKey: apiKey,
		from:   from,
		apiURL: apiURL,
		client: &http.Client{Timeout: requestTimeout},
	}
}

type sendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

// Send delivers one email. html is the full message body - callers build
// it (see SendInviteEmail for the one message this app currently sends),
// there's no template engine here.
func (s *Service) Send(ctx context.Context, to, subject, html string) error {
	body, err := json.Marshal(sendRequest{
		From:    s.from,
		To:      []string{to},
		Subject: subject,
		HTML:    html,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("resend: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("resend: unexpected status %d: %s", resp.StatusCode, respBody)
	}

	return nil
}

// SendInviteEmail sends the one transactional email this app currently
// has: "you've been approved, here's your sign-up link." signupURL
// should already be the complete, absolute URL (see
// cmd.buildSignupURL) - this package doesn't know the app's own domain.
func (s *Service) SendInviteEmail(ctx context.Context, to, signupURL string) error {
	html := fmt.Sprintf(`<p>You've been invited to join mishis4x.</p>
<p><a href="%s">Click here to create your account</a></p>
<p>This link only works once.</p>`, signupURL)

	return s.Send(ctx, to, "You're invited to mishis4x", html)
}
