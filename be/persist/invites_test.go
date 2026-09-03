package persist

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateInvite(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	token, err := p.CreateInvite(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM invites WHERE token = ?", token)
	})

	require.NotEmpty(t, token)
	// 32 random bytes, base64 URL-safe, no padding - same shape as a real
	// session token (see NewSessionToken).
	require.Len(t, token, 43)
}

func TestRedeemInvite_Success(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	token, err := p.CreateInvite(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM invites WHERE token = ?", token)
	})

	require.NoError(t, p.RedeemInvite(t.Context(), token))
}

func TestRedeemInvite_AlreadyUsed(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	token, err := p.CreateInvite(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM invites WHERE token = ?", token)
	})

	require.NoError(t, p.RedeemInvite(t.Context(), token))

	err = p.RedeemInvite(t.Context(), token)
	require.ErrorIs(t, err, ErrInvalidInvite)
}

func TestRedeemInvite_NonExistentToken(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	err := p.RedeemInvite(t.Context(), "this-token-was-never-created")
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

	token, err := p.CreateInvite(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM invites WHERE token = ?", token)
	})

	const attempts = 10
	var wg sync.WaitGroup
	results := make([]error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = p.RedeemInvite(t.Context(), token)
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

func TestMarkInviteUsedBy(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	token, err := p.CreateInvite(t.Context())
	require.NoError(t, err)

	username := fmt.Sprintf("invite-test-user-%d", os.Getpid())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM invites WHERE token = ?", token)
		_, _ = db.Exec("DELETE FROM users WHERE username = ?", username)
	})

	require.NoError(t, p.RedeemInvite(t.Context(), token))

	userID, err := p.CreateUser(t.Context(), User{Username: username, Status: "active", Password: "hashedpw"})
	require.NoError(t, err)

	require.NoError(t, p.MarkInviteUsedBy(t.Context(), token, userID))

	var usedBy int
	require.NoError(t, db.QueryRow("SELECT used_by_user_id FROM invites WHERE token = ?", token).Scan(&usedBy))
	require.Equal(t, userID, usedBy)
}
