package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"example.com/mishis4x/api"
	"example.com/mishis4x/persist"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Status   string `json:"status"`
}

func (d *Data) UserCreate(w http.ResponseWriter, r *http.Request) {
	var u User
	u.Status = "active"

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&u)

	if err != nil {
		log.Error().Err(err).Msg("error decoding user")
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)

	if err != nil {
		log.Error().Err(err).Msg("error hashing password")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	id, err := d.P.CreateUser(ctx, persist.User{
		Username: u.Username,
		Password: string(hashedPassword),
		Status:   u.Status,
	})

	if err != nil {
		log.Error().Err(err).Msg("error creating user")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		log.Error().Err(err).Msg("error getting session")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	session.Values["userID"] = id
	session.Values["authenticated"] = true
	// saves cookie
	err = session.Save(r, w)
	if err != nil {
		log.Error().Err(err).Msg("error saving session on create")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusCreated)

	// Deliberately not logging u (contains the plaintext password field).
	log.Info().Str("username", u.Username).Int("userID", id).Msg("new user created")
}

func (d *Data) UserLogin(w http.ResponseWriter, r *http.Request) {
	var b api.UserLogin

	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&b)
	if err != nil {
		log.Error().Err(err).Msg("error decoding user")
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	u, err := d.P.GetUserByUsername(ctx, b.Username)
	if err != nil {
		log.Error().Err(err).Str("username", b.Username).Msg("error getting user")
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	err = bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(b.Password))

	if err != nil {
		log.Warn().Str("username", b.Username).Msg("login failed: password mismatch")
		http.Error(w, err.Error(), http.StatusUnauthorized)
	}
	log.Info().Str("username", u.Username).Msg("user authenticated")
	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		log.Error().Err(err).Msg("error getting session")
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	session.Values["userID"] = u.ID
	session.Values["authenticated"] = true
	err = session.Save(r, w)
	if err != nil {
		log.Error().Err(err).Msg("error saving session on login")
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

// TODO: maybe move to its own file ?
func (d *Data) UserLogout(w http.ResponseWriter, r *http.Request) {
	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	session.Values["authenticated"] = false
	session.Options.MaxAge = -1
	err = session.Save(r, w)
	if err != nil {
		log.Error().Err(err).Msg("error saving session on logout")
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
