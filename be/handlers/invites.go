package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"example.com/mishis4x/persist"
	"github.com/rs/zerolog/log"
)

const maxEmailLen = 255

type inviteRequestBody struct {
	EmailAddress string `json:"email_address"`
}

// validateEmailAddress is shared by every entry point that takes a raw,
// user-typed email address (RequestInvite, RequestPasswordReset) -
// UserCreate doesn't need its own copy, since signup only ever uses the
// email already attached to an invite record, which went through this
// same check when the invite was requested.
func validateEmailAddress(emailAddress string) string {
	switch {
	case emailAddress == "":
		return "Email address is required."
	case len(emailAddress) > maxEmailLen:
		return "Email address is too long."
	}

	// mail.ParseAddress also accepts "Name <addr@example.com>" - only
	// accept the bare address form, not that display-name syntax.
	parsed, err := mail.ParseAddress(emailAddress)
	if err != nil || parsed.Address != emailAddress {
		return "Please enter a valid email address."
	}

	// mail.ParseAddress is pure RFC 5322 *syntax* parsing, not real-world
	// domain shape - RFC 5322's addr-spec grammar genuinely permits a
	// single-label domain with no dot at all, so "someone@gmail" (missing
	// its own ".com") parses without error. Requiring a dot, with at
	// least two characters after the last one (every real TLD is 2+
	// chars), catches that without a DNS/MX lookup - not a guarantee the
	// address is real or deliverable, just a cheap, obvious typo this
	// would otherwise let straight through.
	domain := emailAddress[strings.LastIndex(emailAddress, "@")+1:]
	lastDot := strings.LastIndex(domain, ".")
	if lastDot == -1 || len(domain)-lastDot-1 < 2 {
		return "Please enter a valid email address."
	}

	return ""
}

// RequestInvite is the public, unauthenticated entry point for someone
// asking to join - it only ever creates a 'requested' row (see
// persist.CreateInviteRequest); nothing here reveals a redeemable code
// to anyone. The owner reviews pending requests with `be invite-list`
// and decides via invite-approve/invite-deny (be/cmd/invite.go) -
// approving is the only thing that actually sends an email.
func (d *Data) RequestInvite(w http.ResponseWriter, r *http.Request) {
	var body inviteRequestBody
	if !decodeJSONBody(w, r, &body) {
		return
	}

	body.EmailAddress = strings.TrimSpace(body.EmailAddress)
	if msg := validateEmailAddress(body.EmailAddress); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}

	// Rate limited per email address - this endpoint is public and
	// unauthenticated, so it's the one place in the app anyone can hit
	// with no account at all. Checked before touching the DB.
	if d.InviteRequestLimiter.locked(body.EmailAddress) {
		log.Warn().Str("email", body.EmailAddress).Msg("invite request blocked: too many attempts")
		writeJSONError(w, http.StatusTooManyRequests, "Too many attempts. Please try again in a few minutes.")
		return
	}
	d.InviteRequestLimiter.recordFailure(body.EmailAddress)

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	err := d.P.CreateInviteRequest(ctx, body.EmailAddress)
	if err != nil && !errors.Is(err, persist.ErrInviteRequestExists) {
		log.Error().Err(err).Msg("error creating invite request")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong submitting your request. Please try again.")
		return
	}

	// Only a genuinely new request notifies - not a duplicate of one
	// already pending (err is ErrInviteRequestExists), so someone
	// impatiently resubmitting doesn't re-notify the admin for a request
	// they've already seen. Best-effort and never affects the response
	// below either way: this is a nice-to-have (the admin can always
	// still find it via `be invite-list`/the /admin page on their own),
	// not something a real signup request should ever fail over. Skips
	// silently, not even a log line, when email/AppBaseURL/
	// AdminNotificationEmail aren't all configured - same as
	// ApproveInviteRequest's own degrade, just non-fatal here since
	// nothing here is admin-initiated.
	if err == nil && d.EmailService != nil && d.AppBaseURL != "" && d.AdminNotificationEmail != "" {
		adminURL := d.AppBaseURL + "/admin"
		if sendErr := d.EmailService.SendInviteRequestNotification(ctx, d.AdminNotificationEmail, body.EmailAddress, adminURL); sendErr != nil {
			log.Error().Err(sendErr).Str("email", body.EmailAddress).Msg("invite request recorded, but the admin notification email failed to send")
		}
	}

	// Same generic response whether this was a brand new request or a
	// duplicate of one already pending - not confirming/denying whether
	// a given address already has an outstanding request.
	w.WriteHeader(http.StatusCreated)
}
