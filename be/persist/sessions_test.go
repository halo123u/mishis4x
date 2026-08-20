package persist

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSessionLifecycle(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	username := fmt.Sprintf("session-test-user-%d", os.Getpid())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM users WHERE username = ?", username)
	})

	userID, err := p.CreateUser(t.Context(), User{Username: username, Status: "active", Password: "hashedpw"})
	require.NoError(t, err)

	session, err := p.CreateSession(t.Context(), userID, time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, session.ID)
	require.Equal(t, userID, session.UserID)

	fetched, err := p.GetSession(t.Context(), session.ID)
	require.NoError(t, err)
	require.Equal(t, session.ID, fetched.ID)
	require.Equal(t, userID, fetched.UserID)

	require.NoError(t, p.DeleteSession(t.Context(), session.ID))

	_, err = p.GetSession(t.Context(), session.ID)
	require.ErrorIs(t, err, ErrSessionNotFound)
}

func TestSession_Expired(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	username := fmt.Sprintf("session-expiry-test-user-%d", os.Getpid())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM users WHERE username = ?", username)
	})

	userID, err := p.CreateUser(t.Context(), User{Username: username, Status: "active", Password: "hashedpw"})
	require.NoError(t, err)

	// A negative TTL creates a session that's already expired.
	session, err := p.CreateSession(t.Context(), userID, -time.Hour)
	require.NoError(t, err)

	_, err = p.GetSession(t.Context(), session.ID)
	require.ErrorIs(t, err, ErrSessionNotFound)
}

func TestSession_NotFound(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	_, err := p.GetSession(t.Context(), "does-not-exist")
	require.ErrorIs(t, err, ErrSessionNotFound)
}

func TestDeleteOtherSessions(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	username := fmt.Sprintf("session-revoke-test-user-%d", os.Getpid())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM users WHERE username = ?", username)
	})

	userID, err := p.CreateUser(t.Context(), User{Username: username, Status: "active", Password: "hashedpw"})
	require.NoError(t, err)

	keep, err := p.CreateSession(t.Context(), userID, time.Hour)
	require.NoError(t, err)
	other1, err := p.CreateSession(t.Context(), userID, time.Hour)
	require.NoError(t, err)
	other2, err := p.CreateSession(t.Context(), userID, time.Hour)
	require.NoError(t, err)

	require.NoError(t, p.DeleteOtherSessions(t.Context(), userID, keep.ID))

	_, err = p.GetSession(t.Context(), keep.ID)
	require.NoError(t, err, "the session passed as keepToken must survive")

	_, err = p.GetSession(t.Context(), other1.ID)
	require.ErrorIs(t, err, ErrSessionNotFound)

	_, err = p.GetSession(t.Context(), other2.ID)
	require.ErrorIs(t, err, ErrSessionNotFound)
}
