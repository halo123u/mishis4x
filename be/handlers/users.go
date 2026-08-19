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

// validateSignupInput returns a user-facing message if username/password
// don't meet the signup requirements, or "" if they're valid. Login
// deliberately does not use this - a login attempt against an existing
// account (e.g. one seeded before these rules existed) must never be
// rejected for being "too short".
func validateSignupInput(username, password string) string {
	switch {
	case username == "":
		return "Username is required."
	case len(username) < minUsernameLen || len(username) > maxUsernameLen:
		return "Username must be between 3 and 32 characters."
	case password == "":
		return "Password is required."
	case len(password) < minPasswordLen:
		return "Password must be at least 8 characters."
	case len(password) > maxPasswordLen:
		return "Password must be at most 72 characters."
	}
	return ""
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

	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		log.Error().Err(err).Msg("error getting session")
		writeJSONError(w, http.StatusInternalServerError, "Your account was created, but we couldn't sign you in automatically. Please log in.")
		return
	}

	session.Values["userID"] = id
	session.Values["authenticated"] = true
	// saves cookie
	if err := session.Save(r, w); err != nil {
		log.Error().Err(err).Msg("error saving session on create")
		writeJSONError(w, http.StatusInternalServerError, "Your account was created, but we couldn't sign you in automatically. Please log in.")
		return
	}

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
		writeJSONError(w, http.StatusUnauthorized, "Incorrect username or password.")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(b.Password)); err != nil {
		log.Warn().Str("username", b.Username).Msg("login failed: password mismatch")
		writeJSONError(w, http.StatusUnauthorized, "Incorrect username or password.")
		return
	}

	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		log.Error().Err(err).Msg("error getting session")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}

	session.Values["userID"] = u.ID
	session.Values["authenticated"] = true
	if err := session.Save(r, w); err != nil {
		log.Error().Err(err).Msg("error saving session on login")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}

	log.Info().Str("username", u.Username).Msg("user authenticated")
}

// TODO: maybe move to its own file ?
func (d *Data) UserLogout(w http.ResponseWriter, r *http.Request) {
	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		log.Error().Err(err).Msg("error getting session")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}

	session.Values["authenticated"] = false
	session.Options.MaxAge = -1
	if err := session.Save(r, w); err != nil {
		log.Error().Err(err).Msg("error saving session on logout")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}
}
