package persist

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

var testPasswordResetUserCounter int

// testPasswordResetUser creates a real user row (password_resets.user_id
// FKs to users(id)) and registers best-effort cleanup - matching
// sessions_test.go's own convention of not bothering to clean up rows
// this package's own functions already delete along the way.
func testPasswordResetUser(t *testing.T, p *Persist) int {
	t.Helper()
	testPasswordResetUserCounter++
	username := fmt.Sprintf("pwreset-test-user-%d-%d", os.Getpid(), testPasswordResetUserCounter)
	t.Cleanup(func() {
		_, _ = p.DB.Exec("DELETE FROM password_resets WHERE user_id = (SELECT id FROM users WHERE username = ?)", username)
		_, _ = p.DB.Exec("DELETE FROM users WHERE username = ?", username)
	})

	userID, err := p.CreateUser(t.Context(), User{Username: username, Status: "active", Password: "hashedpw"})
	require.NoError(t, err)
	return userID
}

func TestCreatePasswordReset(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := testPasswordResetUser(t, p)

	token, err := p.CreatePasswordReset(t.Context(), userID)
	require.NoError(t, err)
	// 32 random bytes, base64 URL-safe, no padding - same shape as a real
	// session token/invite code.
	require.Len(t, token, 43)

	gotUserID, err := p.ConsumePasswordReset(t.Context(), token)
	require.NoError(t, err)
	require.Equal(t, userID, gotUserID)
}

func TestCreatePasswordReset_InvalidatesEarlierUnusedReset(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := testPasswordResetUser(t, p)

	first, err := p.CreatePasswordReset(t.Context(), userID)
	require.NoError(t, err)

	// A second request (e.g. someone hit "forgot password" twice) must
	// invalidate the first link - only the most recently requested one
	// should ever work.
	second, err := p.CreatePasswordReset(t.Context(), userID)
	require.NoError(t, err)
	require.NotEqual(t, first, second)

	_, err = p.ConsumePasswordReset(t.Context(), first)
	require.ErrorIs(t, err, ErrPasswordResetInvalid, "the earlier link must no longer work")

	gotUserID, err := p.ConsumePasswordReset(t.Context(), second)
	require.NoError(t, err)
	require.Equal(t, userID, gotUserID)
}

func TestConsumePasswordReset_AlreadyUsed(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := testPasswordResetUser(t, p)

	token, err := p.CreatePasswordReset(t.Context(), userID)
	require.NoError(t, err)

	_, err = p.ConsumePasswordReset(t.Context(), token)
	require.NoError(t, err)

	// Single-use - a second submission of the same link (e.g. a page
	// reload, or a real double-spend attempt) must not succeed again.
	_, err = p.ConsumePasswordReset(t.Context(), token)
	require.ErrorIs(t, err, ErrPasswordResetInvalid)
}

func TestConsumePasswordReset_Expired(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := testPasswordResetUser(t, p)

	token, err := p.CreatePasswordReset(t.Context(), userID)
	require.NoError(t, err)

	// Back-date it past its TTL directly - passwordResetTTL is only an
	// hour, not practical to actually wait out in a test.
	_, err = db.Exec("UPDATE password_resets SET expires_at = NOW() - INTERVAL 1 MINUTE WHERE token = ?", token)
	require.NoError(t, err)

	_, err = p.ConsumePasswordReset(t.Context(), token)
	require.ErrorIs(t, err, ErrPasswordResetInvalid)
}

func TestConsumePasswordReset_NonExistentToken(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	_, err := p.ConsumePasswordReset(t.Context(), "this-token-was-never-created")
	require.ErrorIs(t, err, ErrPasswordResetInvalid)
}
