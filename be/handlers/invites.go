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

	// Same generic response whether this was a brand new request or a
	// duplicate of one already pending - not confirming/denying whether
	// a given address already has an outstanding request.
	w.WriteHeader(http.StatusCreated)
}
