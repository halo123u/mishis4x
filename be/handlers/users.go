package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"example.com/mishis4x/api"
	"example.com/mishis4x/persist"
	"github.com/go-sql-driver/mysql"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

const (
	minUsernameLen = 3
	maxUsernameLen = 32
	minPasswordLen = 8
	// bcrypt silently ignores input past 72 bytes - reject before that
	// point instead of accepting a password that's partially useless.
	maxPasswordLen = 72

	// MySQL "Duplicate entry" error number, used to distinguish "username
	// already taken" from any other insert failure.
	mysqlErrDuplicateEntry = 1062
)

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Status   string `json:"status"`
	// InviteToken gates signup - see UserCreate's doc comment. Not part
	// of persist.User (it's redeemed, not stored on the user record
	// itself - see persist.MarkInviteUsedBy for where it does get
	// recorded, on the invites row instead).
	InviteToken string `json:"invite_token"`
}

// validatePassword is shared by signup and change-password - login
// deliberately does not use it, so an existing/seeded account is never
// locked out for being "too short" by rules added after it was created.
func validatePassword(password string) string {
	switch {
	case password == "":
		return "Password is required."
	case len(password) < minPasswordLen:
		return "Password must be at least 8 characters."
	case len(password) > maxPasswordLen:
		return "Password must be at most 72 characters."
	}
	return ""
}

func validateSignupInput(username, password string) string {
	switch {
	case username == "":
		return "Username is required."
	case len(username) < minUsernameLen || len(username) > maxUsernameLen:
		return "Username must be between 3 and 32 characters."
	}
	return validatePassword(password)
}

// UserCreate is invite-only, not open public signup - a valid, unused
// invites row (see persist.RedeemInvite) is required before an account
// gets created. Invites are minted by the app owner via `be invite
// create` (be/cmd/invite.go), not through any API endpoint - there's no
// "invite someone else" feature for a regular user, matching this app's
// current single-owner-controlled-access shape (same spirit as
// CollectionOwnerUserID elsewhere).
func (d *Data) UserCreate(w http.ResponseWriter, r *http.Request) {
	var u User
	u.Status = "active"

	if !decodeJSONBody(w, r, &u) {
		return
	}

	u.Username = strings.TrimSpace(u.Username)

	if msg := validateSignupInput(u.Username, u.Password); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	// Rate limited per username, separately from login (see
	// attemptLimiter's doc comment) - mainly to slow down probing whether a
	// given username is taken via repeated signup attempts against it.
	// Checked before the invite gets burned below: a username already
	// under lockout is going to be rejected regardless of invite
	// validity, so there's no reason to spend a scarce, hand-issued
	// invite on a request that can't succeed anyway.
	if d.SignupLimiter.locked(u.Username) {
		log.Warn().Str("username", u.Username).Msg("signup blocked: too many failed attempts")
		writeJSONError(w, http.StatusTooManyRequests, "Too many attempts. Please try again in a few minutes.")
		return
	}

	// Claimed before the bcrypt work below, so an invalid or already-used
	// token fails as cheaply as possible - see persist.RedeemInvite's doc
	// comment for why this is safe under concurrent redemption attempts
	// and why a signup that fails further down still burns the invite
	// rather than un-claiming it.
	if err := d.P.RedeemInvite(ctx, u.InviteToken); err != nil {
		if errors.Is(err, persist.ErrInvalidInvite) {
			log.Warn().Msg("signup blocked: invalid or already-used invite")
			writeJSONError(w, http.StatusForbidden, "This invite link is invalid or has already been used.")
			return
		}
		log.Error().Err(err).Msg("error redeeming invite")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong creating your account. Please try again.")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Error().Err(err).Msg("error hashing password")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong creating your account. Please try again.")
		return
	}

	id, err := d.P.CreateUser(ctx, persist.User{
		Username: u.Username,
		Password: string(hashedPassword),
		Status:   u.Status,
	})
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrDuplicateEntry {
			log.Warn().Str("username", u.Username).Msg("signup failed: username already taken")
			d.SignupLimiter.recordFailure(u.Username)
			writeJSONError(w, http.StatusConflict, "That username is already taken.")
			return
		}
		log.Error().Err(err).Msg("error creating user")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong creating your account. Please try again.")
		return
	}
	d.SignupLimiter.recordSuccess(u.Username)

	// Best-effort - see MarkInviteUsedBy's doc comment for why a failure
	// here shouldn't fail a signup that already succeeded.
	if err := d.P.MarkInviteUsedBy(ctx, u.InviteToken, id); err != nil {
		log.Error().Err(err).Int("userID", id).Msg("error recording invite redeemer")
	}

	session, err := d.P.CreateSession(ctx, id, d.Sessions.TTL)
	if err != nil {
		log.Error().Err(err).Msg("error creating session")
		writeJSONError(w, http.StatusInternalServerError, "Your account was created, but we couldn't sign you in automatically. Please log in.")
		return
	}
	d.setSessionCookie(w, session.ID)

	w.WriteHeader(http.StatusCreated)

	// Deliberately not logging u (contains the plaintext password field).
	log.Info().Str("username", u.Username).Int("userID", id).Msg("new user created")
}

func (d *Data) UserLogin(w http.ResponseWriter, r *http.Request) {
	var b api.UserLogin

	if !decodeJSONBody(w, r, &b) {
		return
	}

	b.Username = strings.TrimSpace(b.Username)
	if b.Username == "" || b.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "Username and password are required.")
		return
	}

	if d.LoginLimiter.locked(b.Username) {
		log.Warn().Str("username", b.Username).Msg("login blocked: too many failed attempts")
		writeJSONError(w, http.StatusTooManyRequests, "Too many failed attempts. Please try again in a few minutes.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	u, err := d.P.GetUserByUsername(ctx, b.Username)
	if err != nil {
		// Same generic response either way - don't let an attacker use this
		// endpoint to enumerate which usernames exist.
		if errors.Is(err, persist.ErrUserNotFound) {
			log.Warn().Str("username", b.Username).Msg("login failed: user not found")
		} else {
			log.Error().Err(err).Str("username", b.Username).Msg("error getting user")
		}
		d.LoginLimiter.recordFailure(b.Username)
		writeJSONError(w, http.StatusUnauthorized, "Incorrect username or password.")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(b.Password)); err != nil {
		log.Warn().Str("username", b.Username).Msg("login failed: password mismatch")
		d.LoginLimiter.recordFailure(b.Username)
		writeJSONError(w, http.StatusUnauthorized, "Incorrect username or password.")
		return
	}

	session, err := d.P.CreateSession(ctx, u.ID, d.Sessions.TTL)
	if err != nil {
		log.Error().Err(err).Msg("error creating session")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}
	d.setSessionCookie(w, session.ID)

	d.LoginLimiter.recordSuccess(b.Username)
	log.Info().Str("username", u.Username).Msg("user authenticated")
}

// TODO: maybe move to its own file ?
func (d *Data) UserLogout(w http.ResponseWriter, r *http.Request) {
	if token := d.sessionToken(r); token != "" {
		ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
		defer cancel()

		if err := d.P.DeleteSession(ctx, token); err != nil {
			log.Error().Err(err).Msg("error deleting session")
			// Still clear the cookie below even if the DB delete failed -
			// logout should never visibly fail from the user's perspective.
		}
	}

	d.clearSessionCookie(w)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// ChangePassword requires the caller's current password (confirms it's
// really them, not just someone with a live session) before accepting a new
// one, then revokes every other session for this user - if a stolen
// password is what prompted the change, this logs out whoever had it.
func (d *Data) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "You must be logged in.")
		return
	}

	var req changePasswordRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if msg := validatePassword(req.NewPassword); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	user, err := d.P.GetUserByID(ctx, userID)
	if err != nil {
		log.Error().Err(err).Int("userID", userID).Msg("error getting user for password change")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.CurrentPassword)); err != nil {
		log.Warn().Int("userID", userID).Msg("change-password failed: current password mismatch")
		writeJSONError(w, http.StatusUnauthorized, "Current password is incorrect.")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Error().Err(err).Msg("error hashing new password")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}

	if err := d.P.UpdateUserPassword(ctx, userID, string(hashedPassword)); err != nil {
		log.Error().Err(err).Int("userID", userID).Msg("error updating password")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}

	if currentToken := d.sessionToken(r); currentToken != "" {
		if err := d.P.DeleteOtherSessions(ctx, userID, currentToken); err != nil {
			log.Error().Err(err).Int("userID", userID).Msg("error revoking other sessions")
		}
	}

	log.Info().Int("userID", userID).Msg("password changed")
}
