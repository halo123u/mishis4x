package persist

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rs/zerolog/log"
)

// passwordResetTTL is how long a requested reset link stays redeemable -
// short, since this is meant to be used within the next few minutes of
// receiving the email, not treated as a standing credential the way a
// session or an invite code is.
const passwordResetTTL = 1 * time.Hour

// NewPasswordResetToken generates a random, URL-safe reset token - same
// shape/entropy as NewSessionToken/NewInviteCode (32 bytes from
// crypto/rand, base64 URL-safe). Kept as its own function rather than
// reused directly since all three mean different things even though the
// implementation happens to be identical.
func NewPasswordResetToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreatePasswordReset issues a fresh reset token for userID, first
// deleting any of that user's existing unused resets - only the most
// recently requested link should ever work, so an old "forgot password"
// email from earlier (that the user may have already found and ignored,
// or that leaked into an inbox they no longer trust) can't still be
// redeemed after a newer one was requested. Wrapped in a transaction so
// the delete and insert are atomic - a crash between the two shouldn't be
// able to leave userID with zero valid resets when CreatePasswordReset
// itself just "succeeded".
func (p *Persist) CreatePasswordReset(ctx context.Context, userID int) (string, error) {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			log.Error().Err(rollbackErr).Msg("error rolling back create password reset transaction")
		}
	}()

	if _, err := sq.Delete("password_resets").
		Where(sq.Eq{"user_id": userID}).
		Where(sq.Expr("used_at IS NULL")).
		RunWith(tx).
		ExecContext(ctx); err != nil {
		return "", err
	}

	token, err := NewPasswordResetToken()
	if err != nil {
		return "", err
	}

	if _, err := sq.Insert("password_resets").
		Columns("token", "user_id", "expires_at").
		Values(token, userID, time.Now().Add(passwordResetTTL)).
		RunWith(tx).
		ExecContext(ctx); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return token, nil
}

// ErrPasswordResetInvalid covers "no such token", "already used", and
// "expired" alike - ConsumePasswordReset's caller-facing error is the same
// either way ("invalid or expired link"), so there's no reason to
// distinguish them and leak which case it was to an unauthenticated
// caller (the same posture RedeemInvite's ErrInvalidInvite takes).
var ErrPasswordResetInvalid = errors.New("invalid or expired password reset token")

// ConsumePasswordReset atomically claims token if (and only if) it exists,
// hasn't been used yet, and hasn't expired - the UPDATE's own WHERE clause
// is the actual concurrency guard (two simultaneous submissions of the
// same reset link can't both succeed), not a check-then-write done in
// application code, the same shape RedeemInvite already uses for its own
// single-use code. Returns the user_id to reset the password for.
func (p *Persist) ConsumePasswordReset(ctx context.Context, token string) (int, error) {
	res, err := sq.Update("password_resets").
		Set("used_at", sq.Expr("NOW()")).
		Where(sq.Eq{"token": token}).
		Where(sq.Expr("used_at IS NULL")).
		Where(sq.Expr("expires_at > NOW()")).
		RunWith(p.DB).
		ExecContext(ctx)
	if err != nil {
		return 0, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rows == 0 {
		return 0, ErrPasswordResetInvalid
	}

	var userID int
	err = sq.Select("user_id").
		From("password_resets").
		Where(sq.Eq{"token": token}).
		RunWith(p.DB).
		QueryRowContext(ctx).
		Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrPasswordResetInvalid
		}
		return 0, err
	}

	return userID, nil
}
