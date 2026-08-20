package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoginLimiter_LocksOutAfterThreshold(t *testing.T) {
	l := newLoginLimiter()

	for i := 0; i < maxFailedLoginAttempts-1; i++ {
		l.recordFailure("alice")
		require.False(t, l.locked("alice"), "should not lock out before the threshold")
	}

	l.recordFailure("alice")
	require.True(t, l.locked("alice"), "should lock out once the threshold is reached")
}

func TestLoginLimiter_TracksUsersIndependently(t *testing.T) {
	l := newLoginLimiter()

	for i := 0; i < maxFailedLoginAttempts; i++ {
		l.recordFailure("alice")
	}

	require.True(t, l.locked("alice"))
	require.False(t, l.locked("bob"), "a different username must not be affected")
}

func TestLoginLimiter_SuccessClearsFailures(t *testing.T) {
	l := newLoginLimiter()

	for i := 0; i < maxFailedLoginAttempts-1; i++ {
		l.recordFailure("alice")
	}

	l.recordSuccess("alice")

	l.recordFailure("alice")
	require.False(t, l.locked("alice"), "a success should reset the failure count")
}

func TestLoginLimiter_UnlocksAfterLockoutDuration(t *testing.T) {
	l := newLoginLimiter()

	now := time.Now()
	l.attempts["alice"] = &loginAttempt{
		count:       maxFailedLoginAttempts,
		windowStart: now.Add(-loginLockoutWindow / 2),
		lockedUntil: now.Add(-time.Second), // already in the past
	}

	require.False(t, l.locked("alice"), "lockout should have expired")
}
