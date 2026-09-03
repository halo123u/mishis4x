package persist

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// testEmail returns a unique-per-call email address so tests don't
// collide with each other or with ErrInviteRequestExists' dedup check.
var testEmailCounter int

func testEmail(t *testing.T) string {
	t.Helper()
	testEmailCounter++
	return fmt.Sprintf("invite-test-%d-%d@example.com", os.Getpid(), testEmailCounter)
}

func TestCreateInviteRequest(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	email := testEmail(t)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM invites WHERE email_address = ?", email) })

	require.NoError(t, p.CreateInviteRequest(t.Context(), email))

	requests, err := p.ListRequestedInvites(t.Context())
	require.NoError(t, err)

	var found *InviteRequest
	for i := range requests {
		if requests[i].EmailAddress == email {
			found = &requests[i]
		}
	}
	require.NotNil(t, found, "newly created request should show up in ListRequestedInvites")
	require.Equal(t, InviteStatusRequested, found.Status)
	// 32 random bytes, base64 URL-safe, no padding - same shape as a real
	// session token (see NewSessionToken) - generated up front at request
	// time, not deferred to approval.
	require.Len(t, found.Code, 43)
}

func TestCreateInviteRequest_DuplicateBlocked(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	email := testEmail(t)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM invites WHERE email_address = ?", email) })

	require.NoError(t, p.CreateInviteRequest(t.Context(), email))
	err := p.CreateInviteRequest(t.Context(), email)
	require.ErrorIs(t, err, ErrInviteRequestExists)
}

func TestCreateInviteRequest_AllowedAfterDenied(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	email := testEmail(t)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM invites WHERE email_address = ?", email) })

	require.NoError(t, p.CreateInviteRequest(t.Context(), email))
	requests, err := p.ListRequestedInvites(t.Context())
	require.NoError(t, err)
	var id int
	for _, r := range requests {
		if r.EmailAddress == email {
			id = r.ID
		}
	}
	require.NotZero(t, id)

	_, err = p.DenyInvite(t.Context(), id)
	require.NoError(t, err)

	// A denied request doesn't permanently block that address - situations
	// change, a fresh request should be allowed.
	require.NoError(t, p.CreateInviteRequest(t.Context(), email))
}

func requestedInvite(t *testing.T, p *Persist, email string) InviteRequest {
	t.Helper()
	require.NoError(t, p.CreateInviteRequest(t.Context(), email))
	requests, err := p.ListRequestedInvites(t.Context())
	require.NoError(t, err)
	for _, r := range requests {
		if r.EmailAddress == email {
			return r
		}
	}
	t.Fatalf("just-created request for %s not found in ListRequestedInvites", email)
	return InviteRequest{}
}

func TestApproveInvite(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	email := testEmail(t)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM invites WHERE email_address = ?", email) })

	req := requestedInvite(t, p, email)

	approved, err := p.ApproveInvite(t.Context(), req.ID)
	require.NoError(t, err)
	require.Equal(t, InviteStatusApproved, approved.Status)
	require.Equal(t, req.Code, approved.Code)
}

func TestApproveInvite_AlreadyDecided(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	email := testEmail(t)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM invites WHERE email_address = ?", email) })

	req := requestedInvite(t, p, email)
	_, err := p.ApproveInvite(t.Context(), req.ID)
	require.NoError(t, err)

	_, err = p.ApproveInvite(t.Context(), req.ID)
	require.ErrorIs(t, err, ErrInviteNotPending)
}

func TestApproveInvite_NonExistentID(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	_, err := p.ApproveInvite(t.Context(), -1)
	require.ErrorIs(t, err, ErrInviteNotPending)
}

func TestDenyInvite(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	email := testEmail(t)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM invites WHERE email_address = ?", email) })

	req := requestedInvite(t, p, email)

	denied, err := p.DenyInvite(t.Context(), req.ID)
	require.NoError(t, err)
	require.Equal(t, InviteStatusDenied, denied.Status)

	// A denied code was never approved, so it must never redeem either.
	_, err = p.RedeemInvite(t.Context(), req.Code)
	require.ErrorIs(t, err, ErrInvalidInvite)
}

func TestRedeemInvite_Success(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	email := testEmail(t)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM invites WHERE email_address = ?", email) })

	req := requestedInvite(t, p, email)
	_, err := p.ApproveInvite(t.Context(), req.ID)
	require.NoError(t, err)

	gotEmail, err := p.RedeemInvite(t.Context(), req.Code)
	require.NoError(t, err)
	require.Equal(t, email, gotEmail)
}

func TestRedeemInvite_NotYetApproved(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	email := testEmail(t)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM invites WHERE email_address = ?", email) })

	req := requestedInvite(t, p, email)

	// Still 'requested', never approved - must not redeem.
	_, err := p.RedeemInvite(t.Context(), req.Code)
	require.ErrorIs(t, err, ErrInvalidInvite)
}

func TestRedeemInvite_AlreadyRedeemed(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	email := testEmail(t)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM invites WHERE email_address = ?", email) })

	req := requestedInvite(t, p, email)
	_, err := p.ApproveInvite(t.Context(), req.ID)
	require.NoError(t, err)

	_, err = p.RedeemInvite(t.Context(), req.Code)
	require.NoError(t, err)

	_, err = p.RedeemInvite(t.Context(), req.Code)
	require.ErrorIs(t, err, ErrInvalidInvite)
}

func TestRedeemInvite_NonExistentCode(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	_, err := p.RedeemInvite(t.Context(), "this-code-was-never-created")
	require.ErrorIs(t, err, ErrInvalidInvite)
}

// TestRedeemInvite_ConcurrentRedemption is the direct proof behind
// RedeemInvite's doc comment claim: the UPDATE's own WHERE clause
// serializes concurrent redemption attempts at the database level, so
// exactly one of many simultaneous callers can ever succeed - not a
// property that's safe to just assume without testing it for real.
func TestRedeemInvite_ConcurrentRedemption(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	email := testEmail(t)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM invites WHERE email_address = ?", email) })

	req := requestedInvite(t, p, email)
	_, err := p.ApproveInvite(t.Context(), req.ID)
	require.NoError(t, err)

	const attempts = 10
	var wg sync.WaitGroup
	results := make([]error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = p.RedeemInvite(t.Context(), req.Code)
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrInvalidInvite)
		}
	}
	require.Equal(t, 1, successes, "exactly one concurrent redemption attempt should succeed")
}

func TestMarkInviteRedeemedBy(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	email := testEmail(t)
	username := fmt.Sprintf("invite-test-user-%d", os.Getpid())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM invites WHERE email_address = ?", email)
		_, _ = db.Exec("DELETE FROM users WHERE username = ?", username)
	})

	req := requestedInvite(t, p, email)
	_, err := p.ApproveInvite(t.Context(), req.ID)
	require.NoError(t, err)
	_, err = p.RedeemInvite(t.Context(), req.Code)
	require.NoError(t, err)

	userID, err := p.CreateUser(t.Context(), User{Username: username, Status: "active", Password: "hashedpw"})
	require.NoError(t, err)

	require.NoError(t, p.MarkInviteRedeemedBy(t.Context(), req.Code, userID))

	var redeemedBy int
	require.NoError(t, db.QueryRow("SELECT redeemed_by_user_id FROM invites WHERE code = ?", req.Code).Scan(&redeemedBy))
	require.Equal(t, userID, redeemedBy)
}
