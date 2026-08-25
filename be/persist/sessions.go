package persist

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

// ErrSessionNotFound is returned by GetSession when the token doesn't match
// any row, or matches one that's expired (an expired session is treated
// identically to a nonexistent one - callers don't need to distinguish).
var ErrSessionNotFound = errors.New("session not found")

type Session struct {
	ID        string
	UserID    int
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewSessionToken generates a random, URL-safe session token: 32 bytes
// (256 bits) from crypto/rand. This token IS the credential - unlike a
// signed cookie, nothing verifies it cryptographically, its own entropy
// (infeasible to guess) plus the server-side row lookup is what secures it.
func NewSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken hashes a raw session token for storage/lookup. The DB never
// holds the raw token, only this hash - a leaked DB row (backup, compromised
// DB user, etc.) is then useless on its own, since the cookie's raw value is
// what's needed to reproduce the hash and match a row. SHA-256 (not bcrypt)
// is the right tool here: the token is already 256 bits of real randomness
// from crypto/rand, not a low-entropy human password, so there's nothing for
// a slow, salted KDF to protect against - a fast general-purpose hash is
// fine, and it keeps every session lookup cheap.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateSession generates a new session token for userID and persists it,
// valid for ttl. The returned Session.ID is the raw token (what the caller
// puts in the cookie) - only its hash is written to the DB.
func (p *Persist) CreateSession(ctx context.Context, userID int, ttl time.Duration) (Session, error) {
	token, err := NewSessionToken()
	if err != nil {
		return Session{}, err
	}

	s := Session{
		ID:        token,
		UserID:    userID,
		ExpiresAt: time.Now().Add(ttl),
	}

	q := `INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?);`
	if _, err := p.DB.ExecContext(ctx, q, hashToken(token), s.UserID, s.ExpiresAt); err != nil {
		return Session{}, err
	}

	return s, nil
}

// GetSession looks up a session by its raw token (hashed before querying -
// see hashToken). Returns ErrSessionNotFound if the token doesn't exist or
// has expired.
func (p *Persist) GetSession(ctx context.Context, token string) (Session, error) {
	q := `SELECT id, user_id, created_at, expires_at FROM sessions WHERE id = ?;`

	var s Session
	err := p.DB.QueryRowContext(ctx, q, hashToken(token)).Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, err
	}

	if time.Now().After(s.ExpiresAt) {
		return Session{}, ErrSessionNotFound
	}

	return s, nil
}

// DeleteSession removes a session by its raw token - this is what makes
// logout an actual server-side revocation instead of just clearing a client
// cookie.
func (p *Persist) DeleteSession(ctx context.Context, token string) error {
	_, err := p.DB.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?;`, hashToken(token))
	return err
}

// DeleteOtherSessions removes every session for userID except the one
// belonging to keepToken (a raw token, hashed before comparing). Used on
// password change: any other logged-in session (e.g. a device you no longer
// trust) gets kicked out, while the session making the change stays valid.
func (p *Persist) DeleteOtherSessions(ctx context.Context, userID int, keepToken string) error {
	q := `DELETE FROM sessions WHERE user_id = ? AND id != ?;`
	_, err := p.DB.ExecContext(ctx, q, userID, hashToken(keepToken))
	return err
}
