package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAttemptLimiter_LocksOutAfterThreshold(t *testing.T) {
	l := newAttemptLimiter()

	for i := 0; i < maxFailedAttempts-1; i++ {
		l.recordFailure("alice")
		require.False(t, l.locked("alice"), "should not lock out before the threshold")
	}

	l.recordFailure("alice")
	require.True(t, l.locked("alice"), "should lock out once the threshold is reached")
}

func TestAttemptLimiter_TracksKeysIndependently(t *testing.T) {
	l := newAttemptLimiter()

	for i := 0; i < maxFailedAttempts; i++ {
		l.recordFailure("alice")
	}

	require.True(t, l.locked("alice"))
	require.False(t, l.locked("bob"), "a different key must not be affected")
}

func TestAttemptLimiter_SuccessClearsFailures(t *testing.T) {
	l := newAttemptLimiter()

	for i := 0; i < maxFailedAttempts-1; i++ {
		l.recordFailure("alice")
	}

	l.recordSuccess("alice")

	l.recordFailure("alice")
	require.False(t, l.locked("alice"), "a success should reset the failure count")
}

func TestAttemptLimiter_UnlocksAfterLockoutDuration(t *testing.T) {
	l := newAttemptLimiter()

	now := time.Now()
	l.attempts["alice"] = &trackedAttempt{
		count:       maxFailedAttempts,
		windowStart: now.Add(-attemptLockoutWindow / 2),
		lockedUntil: now.Add(-time.Second), // already in the past
	}

	require.False(t, l.locked("alice"), "lockout should have expired")
}
