package persist

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
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

// CreateSession generates a new session token for userID and persists it,
// valid for ttl.
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
	if _, err := p.DB.ExecContext(ctx, q, s.ID, s.UserID, s.ExpiresAt); err != nil {
		return Session{}, err
	}

	return s, nil
}

// GetSession looks up a session by its token. Returns ErrSessionNotFound if
// the token doesn't exist or has expired.
func (p *Persist) GetSession(ctx context.Context, token string) (Session, error) {
	q := `SELECT id, user_id, created_at, expires_at FROM sessions WHERE id = ?;`

	var s Session
	err := p.DB.QueryRowContext(ctx, q, token).Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.ExpiresAt)
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

// DeleteSession removes a session by token - this is what makes logout an
// actual server-side revocation instead of just clearing a client cookie.
func (p *Persist) DeleteSession(ctx context.Context, token string) error {
	_, err := p.DB.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?;`, token)
	return err
}

// DeleteOtherSessions removes every session for userID except keepToken.
// Used on password change: any other logged-in session (e.g. a device you
// no longer trust) gets kicked out, while the session making the change
// stays valid.
func (p *Persist) DeleteOtherSessions(ctx context.Context, userID int, keepToken string) error {
	q := `DELETE FROM sessions WHERE user_id = ? AND id != ?;`
	_, err := p.DB.ExecContext(ctx, q, userID, keepToken)
	return err
}
