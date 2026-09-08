package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"example.com/mishis4x/persist"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

type passwordResetRequestBody struct {
	EmailAddress string `json:"email_address"`
}

// RequestPasswordReset is the public, unauthenticated "forgot password"
// entry point - takes an email address and, if it matches exactly one
// account with a real email on file (see persist.GetUserByEmail's doc
// comment for why "exactly one" isn't automatic), emails a reset link.
//
// Every response past the initial format/service-configured/rate-limit
// checks is a plain 200, regardless of what actually happened underneath
// (no account found, a lookup error, a send failure) - none of those
// checks are correlated with whether the submitted address has an
// account, so returning a different status for any of them would let this
// endpoint be used to enumerate which emails are registered. Real errors
// are still logged server-side, just never surfaced as a different
// response to the caller. This is RequestInvite's own "same generic
// response whether new or duplicate" posture, carried one step further to
// also cover the steps after the initial lookup.
func (d *Data) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var body passwordResetRequestBody
	if !decodeJSONBody(w, r, &body) {
		return
	}

	body.EmailAddress = strings.TrimSpace(body.EmailAddress)
	if msg := validateEmailAddress(body.EmailAddress); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}

	// Checked before touching the DB, and before the rate limiter, same
	// as ApproveInviteRequest's identical check - sending the email is
	// this endpoint's whole purpose, not a side notification, so an
	// unconfigured server should say so plainly rather than pretending to
	// have sent something it didn't. Not account-existence-correlated
	// (this is a fixed, server-wide state, the same regardless of which
	// email was submitted), so returning a distinct response here doesn't
	// weaken the no-enumeration posture below.
	if d.EmailService == nil || d.AppBaseURL == "" {
		log.Error().Msg("password-reset request: EmailService/AppBaseURL not configured")
		writeJSONError(w, http.StatusServiceUnavailable, "Password reset isn't available right now. Please contact the site owner.")
		return
	}

	if d.PasswordResetLimiter.locked(body.EmailAddress) {
		log.Warn().Str("email", body.EmailAddress).Msg("password reset blocked: too many attempts")
		writeJSONError(w, http.StatusTooManyRequests, "Too many attempts. Please try again in a few minutes.")
		return
	}
	d.PasswordResetLimiter.recordFailure(body.EmailAddress)

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	user, err := d.P.GetUserByEmail(ctx, body.EmailAddress)
	if err != nil {
		if !errors.Is(err, persist.ErrUserNotFound) {
			log.Error().Err(err).Msg("error looking up user for password reset")
		}
		// No account has this email (or it matches more than one, see
		// GetUserByEmail) - same response as a real send below, so this
		// can't be used to enumerate which addresses are registered.
		w.WriteHeader(http.StatusOK)
		return
	}

	token, err := d.P.CreatePasswordReset(ctx, user.ID)
	if err != nil {
		log.Error().Err(err).Int("userID", user.ID).Msg("error creating password reset")
		w.WriteHeader(http.StatusOK)
		return
	}

	resetURL := fmt.Sprintf("%s/reset-password?token=%s", d.AppBaseURL, token)
	if err := d.EmailService.SendPasswordResetEmail(ctx, body.EmailAddress, resetURL); err != nil {
		log.Error().Err(err).Str("email", body.EmailAddress).Msg("password reset created, but the email failed to send")
	}

	w.WriteHeader(http.StatusOK)
}

type passwordResetConfirmBody struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// ConfirmPasswordReset is the public, unauthenticated second half of the
// "forgot password" flow - takes the token from the emailed link plus a
// new password, and if the token is still valid (see
// persist.ConsumePasswordReset), sets it. Unlike RequestPasswordReset,
// there's no enumeration concern here to design around: the token itself
// is the only thing identifying an account, and it's already
// unguessable (32 random bytes) - a real vs. invalid/expired token is a
// perfectly safe thing to distinguish in the response.
//
// Revokes every existing session for the account (not just "other"
// sessions the way ChangePassword does - there's no "current session" to
// preserve here, the caller isn't logged in at all during a reset) before
// issuing a fresh one and logging the caller straight in, matching
// UserCreate's own auto-login-after-signup convenience.
func (d *Data) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var body passwordResetConfirmBody
	if !decodeJSONBody(w, r, &body) {
		return
	}

	if body.Token == "" {
		writeJSONError(w, http.StatusBadRequest, "This reset link is invalid or has expired. Please request a new one.")
		return
	}
	if msg := validatePassword(body.NewPassword); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	userID, err := d.P.ConsumePasswordReset(ctx, body.Token)
	if err != nil {
		if errors.Is(err, persist.ErrPasswordResetInvalid) {
			writeJSONError(w, http.StatusBadRequest, "This reset link is invalid or has expired. Please request a new one.")
			return
		}
		log.Error().Err(err).Msg("error consuming password reset")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Error().Err(err).Msg("error hashing new password")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}

	if err := d.P.UpdateUserPassword(ctx, userID, string(hashedPassword)); err != nil {
		log.Error().Err(err).Int("userID", userID).Msg("error updating password after reset")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}

	if err := d.P.DeleteAllSessions(ctx, userID); err != nil {
		log.Error().Err(err).Int("userID", userID).Msg("error revoking sessions after password reset")
	}

	log.Info().Int("userID", userID).Msg("password reset via email link")

	session, err := d.P.CreateSession(ctx, userID, d.Sessions.TTL)
	if err != nil {
		// Password was already reset successfully at this point - this
		// only means the auto-login convenience didn't work, not that
		// the reset itself failed. The caller can still just log in
		// manually with the new password.
		log.Error().Err(err).Int("userID", userID).Msg("error creating session after password reset")
		w.WriteHeader(http.StatusOK)
		return
	}
	d.setSessionCookie(w, session.ID)

	w.WriteHeader(http.StatusOK)
}
