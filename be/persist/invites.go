package persist

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
)

// Invite status values - see the invites table's own migration doc comment
// for the full state machine (requested -> approved|denied,
// approved -> redeemed). Kept as plain strings (matching users.status'
// convention) rather than a MySQL ENUM, so adding a future status doesn't
// need an ALTER.
const (
	InviteStatusRequested = "requested"
	InviteStatusApproved  = "approved"
	InviteStatusDenied    = "denied"
	InviteStatusRedeemed  = "redeemed"
)

type InviteRequest struct {
	ID           int
	Code         string
	Status       string
	EmailAddress string
	CreatedAt    time.Time
}

// NewInviteCode generates a cryptographically random, URL-safe invite
// code - same shape/entropy as NewSessionToken (32 random bytes, base64
// URL-safe), kept as its own function rather than reused directly since
// the two mean different things even though the implementation happens
// to be identical today.
func NewInviteCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ErrInviteRequestExists is returned by CreateInviteRequest when
// emailAddress already has an outstanding (requested or approved, i.e.
// not yet redeemed) invite - resubmitting the request form for the same
// address shouldn't mint a second code for the owner to have to sort out.
// A prior denied or already-redeemed request doesn't block a fresh one.
var ErrInviteRequestExists = errors.New("an invite request for this email is already pending")

// CreateInviteRequest is the public "request an invite" form's entry
// point - generates the code immediately (not deferred until approval,
// so approving is just a status flip plus sending the email, not a
// second code-generation step) and stores it with status 'requested'.
func (p *Persist) CreateInviteRequest(ctx context.Context, emailAddress string) error {
	var existing int
	err := sq.Select("COUNT(*)").
		From("invites").
		Where(sq.Eq{"email_address": emailAddress, "status": []string{InviteStatusRequested, InviteStatusApproved}}).
		RunWith(p.DB).
		QueryRowContext(ctx).
		Scan(&existing)
	if err != nil {
		return err
	}
	if existing > 0 {
		return ErrInviteRequestExists
	}

	code, err := NewInviteCode()
	if err != nil {
		return err
	}

	_, err = sq.Insert("invites").
		Columns("code", "status", "email_address").
		Values(code, InviteStatusRequested, emailAddress).
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}

// ListRequestedInvites returns every invite still awaiting an
// approve/deny decision, oldest first - what `be invite-list` shows.
func (p *Persist) ListRequestedInvites(ctx context.Context) ([]InviteRequest, error) {
	rows, err := sq.Select("id", "code", "status", "email_address", "created_at").
		From("invites").
		Where(sq.Eq{"status": InviteStatusRequested}).
		OrderBy("created_at ASC").
		RunWith(p.DB).
		QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []InviteRequest
	for rows.Next() {
		var r InviteRequest
		if err := rows.Scan(&r.ID, &r.Code, &r.Status, &r.EmailAddress, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ErrInviteNotPending covers "no such id" and "not currently in
// 'requested' status" for both ApproveInvite and DenyInvite - the id
// came from `be invite-list`'s own output, so either case means it's
// already been decided (possibly by a second concurrent CLI invocation)
// since that listing was printed.
var ErrInviteNotPending = errors.New("invite request not found or already decided")

// ApproveInvite atomically flips a pending request to 'approved' and
// returns the code/email the caller needs to actually send the
// invite email - the UPDATE's own WHERE clause (status = 'requested') is
// what makes this safe to call concurrently with itself or DenyInvite on
// the same id, not a check-then-write in application code.
func (p *Persist) ApproveInvite(ctx context.Context, id int) (InviteRequest, error) {
	return p.decideInvite(ctx, id, InviteStatusApproved)
}

// DenyInvite atomically flips a pending request to 'denied' - no email
// ever gets sent, and the code never leaves the DB.
func (p *Persist) DenyInvite(ctx context.Context, id int) (InviteRequest, error) {
	return p.decideInvite(ctx, id, InviteStatusDenied)
}

func (p *Persist) decideInvite(ctx context.Context, id int, newStatus string) (InviteRequest, error) {
	res, err := sq.Update("invites").
		Set("status", newStatus).
		Where(sq.Eq{"id": id, "status": InviteStatusRequested}).
		RunWith(p.DB).
		ExecContext(ctx)
	if err != nil {
		return InviteRequest{}, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return InviteRequest{}, err
	}
	if rows == 0 {
		return InviteRequest{}, ErrInviteNotPending
	}

	var r InviteRequest
	err = sq.Select("id", "code", "status", "email_address", "created_at").
		From("invites").
		Where(sq.Eq{"id": id}).
		RunWith(p.DB).
		QueryRowContext(ctx).
		Scan(&r.ID, &r.Code, &r.Status, &r.EmailAddress, &r.CreatedAt)
	if err != nil {
		return InviteRequest{}, err
	}
	return r, nil
}

// ErrInvalidInvite covers both "no such code" and "not currently
// approved" (never requested, still pending, denied, or already
// redeemed) - the caller-facing signup error is the same either way
// ("invalid or expired invite"), so there's no reason to distinguish
// them at this layer and leak which case it was to an unauthenticated
// caller.
var ErrInvalidInvite = errors.New("invalid or unredeemable invite")

// RedeemInvite atomically claims code if (and only if) it exists and is
// currently 'approved' - the UPDATE's own WHERE clause is the actual
// concurrency guard (two simultaneous redemptions of the same code can't
// both succeed, MySQL serializes the row-level update), not a
// check-then-write done in application code, which would have a real
// race window between the check and the write. Returns the email address
// on the invite so the caller can copy it onto the new user row.
//
// Deliberately called before the new user row exists (see
// handlers.UserCreate), so this can't set redeemed_by_user_id yet -
// that's filled in by a separate MarkInviteRedeemedBy call once the
// user's actually been created. A signup that fails after this succeeds
// (e.g. a duplicate-username race) burns the code rather than
// un-claiming it - simpler than wrapping both in one transaction, and an
// acceptable tradeoff at this app's scale (deny/re-approve isn't
// possible once redeemed, but a fresh request can always be submitted).
func (p *Persist) RedeemInvite(ctx context.Context, code string) (string, error) {
	res, err := sq.Update("invites").
		Set("status", InviteStatusRedeemed).
		Set("redeemed_at", sq.Expr("NOW()")).
		Where(sq.Eq{"code": code, "status": InviteStatusApproved}).
		RunWith(p.DB).
		ExecContext(ctx)
	if err != nil {
		return "", err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if rows == 0 {
		return "", ErrInvalidInvite
	}

	var email string
	err = sq.Select("email_address").
		From("invites").
		Where(sq.Eq{"code": code}).
		RunWith(p.DB).
		QueryRowContext(ctx).
		Scan(&email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalidInvite
		}
		return "", err
	}

	return email, nil
}

// MarkInviteRedeemedBy records who redeemed code, purely as an audit
// trail - not part of the security gate itself (see the invites table's
// own doc comment). Best-effort: called after RedeemInvite has already
// succeeded and the new user exists, so a failure here shouldn't undo a
// signup that already completed - the caller logs it, doesn't fail the
// request.
func (p *Persist) MarkInviteRedeemedBy(ctx context.Context, code string, userID int) error {
	_, err := sq.Update("invites").
		Set("redeemed_by_user_id", userID).
		Where(sq.Eq{"code": code}).
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}
