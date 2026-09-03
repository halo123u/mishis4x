package persist

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"

	sq "github.com/Masterminds/squirrel"
)

// NewInviteToken generates a cryptographically random, URL-safe invite
// token - same shape/entropy as NewSessionToken (32 random bytes, base64
// URL-safe), kept as its own function rather than reused directly since
// the two mean different things even though the implementation happens
// to be identical today.
func NewInviteToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateInvite mints a new, unused invite token and stores it.
func (p *Persist) CreateInvite(ctx context.Context) (string, error) {
	token, err := NewInviteToken()
	if err != nil {
		return "", err
	}

	_, err = sq.Insert("invites").
		Columns("token").
		Values(token).
		RunWith(p.DB).
		ExecContext(ctx)
	if err != nil {
		return "", err
	}

	return token, nil
}

// ErrInvalidInvite covers both "no such token" and "already used" - the
// caller-facing signup error is the same either way ("invalid or expired
// invite"), so there's no reason to distinguish them at this layer and
// leak which case it was to an unauthenticated caller.
var ErrInvalidInvite = errors.New("invalid or already-used invite")

// RedeemInvite atomically claims token if (and only if) it exists and
// hasn't been used yet - the UPDATE's own WHERE clause is the actual
// concurrency guard (two simultaneous redemptions of the same token can't
// both succeed, MySQL serializes the row-level update), not a
// check-then-write done in application code, which would have a real
// race window between the check and the write.
//
// Deliberately called before the new user row exists (see
// handlers.UserCreate), so this can't set used_by_user_id yet - that's
// filled in by a separate MarkInviteUsedBy call once the user's actually
// been created. A signup that fails after this succeeds (e.g. a
// duplicate-username race) burns the invite rather than un-claiming it -
// simpler than wrapping both in one transaction, and an acceptable
// tradeoff at this app's scale (mint another invite, not a real cost).
func (p *Persist) RedeemInvite(ctx context.Context, token string) error {
	res, err := sq.Update("invites").
		Set("used_at", sq.Expr("NOW()")).
		Where(sq.Eq{"token": token}).
		Where("used_at IS NULL").
		RunWith(p.DB).
		ExecContext(ctx)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrInvalidInvite
	}

	return nil
}

// MarkInviteUsedBy records who redeemed token, purely as an audit trail -
// not part of the security gate itself (see the invites table's own doc
// comment). Best-effort: called after RedeemInvite has already succeeded
// and the new user exists, so a failure here shouldn't undo a signup
// that already completed - the caller logs it, doesn't fail the request.
func (p *Persist) MarkInviteUsedBy(ctx context.Context, token string, userID int) error {
	_, err := sq.Update("invites").
		Set("used_by_user_id", userID).
		Where(sq.Eq{"token": token}).
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}
