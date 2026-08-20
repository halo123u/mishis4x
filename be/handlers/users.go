package handlers

import (
	"context"
	"encoding/json"
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

func (d *Data) UserCreate(w http.ResponseWriter, r *http.Request) {
	var u User
	u.Status = "active"

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&u); err != nil {
		log.Error().Err(err).Msg("error decoding user")
		writeJSONError(w, http.StatusBadRequest, "Invalid request.")
		return
	}

	u.Username = strings.TrimSpace(u.Username)

	if msg := validateSignupInput(u.Username, u.Password); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Error().Err(err).Msg("error hashing password")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong creating your account. Please try again.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	id, err := d.P.CreateUser(ctx, persist.User{
		Username: u.Username,
		Password: string(hashedPassword),
		Status:   u.Status,
	})
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrDuplicateEntry {
			log.Warn().Str("username", u.Username).Msg("signup failed: username already taken")
			writeJSONError(w, http.StatusConflict, "That username is already taken.")
			return
		}
		log.Error().Err(err).Msg("error creating user")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong creating your account. Please try again.")
		return
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

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&b); err != nil {
		log.Error().Err(err).Msg("error decoding login request")
		writeJSONError(w, http.StatusBadRequest, "Invalid request.")
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("error decoding change-password request")
		writeJSONError(w, http.StatusBadRequest, "Invalid request.")
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
