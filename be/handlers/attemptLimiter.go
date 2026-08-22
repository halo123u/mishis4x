package handlers

import (
	"sync"
	"time"
)

const (
	maxFailedAttempts      = 5
	attemptLockoutWindow   = 15 * time.Minute
	attemptLockoutDuration = 5 * time.Minute
)

type trackedAttempt struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

func (a *trackedAttempt) expired(now time.Time) bool {
	return now.Sub(a.windowStart) > attemptLockoutWindow && now.After(a.lockedUntil)
}

// attemptLimiter tracks failed attempts per key (a username, for both login
// and signup - see Data.LoginLimiter/SignupLimiter), in memory. Two separate
// instances are used so a run of signup attempts against an already-taken
// username can't also lock out someone genuinely trying to log into that
// account, and vice versa.
//
// This app runs as a single instance (the same constraint documented for
// matchmaking's in-memory state in CLAUDE.md applies here), so in-memory is
// a deliberate choice: no DB round-trip per attempt, no cleanup job to run -
// at the cost of resetting on restart, an acceptable tradeoff for this
// threat model (slowing down guessing/probing against one key, not building
// a distributed rate limiter).
type attemptLimiter struct {
	mu       sync.Mutex
	attempts map[string]*trackedAttempt
}

func newAttemptLimiter() *attemptLimiter {
	return &attemptLimiter{attempts: make(map[string]*trackedAttempt)}
}

// locked reports whether key is currently locked out.
func (l *attemptLimiter) locked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[key]
	if !ok {
		return false
	}
	return time.Now().Before(a.lockedUntil)
}

// recordFailure records a failed attempt for key, locking it out once it
// crosses maxFailedAttempts within attemptLockoutWindow. Also sweeps fully-
// expired entries out of the map so it doesn't grow unbounded.
func (l *attemptLimiter) recordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	for k, a := range l.attempts {
		if a.expired(now) {
			delete(l.attempts, k)
		}
	}

	a, ok := l.attempts[key]
	if !ok || now.Sub(a.windowStart) > attemptLockoutWindow {
		a = &trackedAttempt{windowStart: now}
		l.attempts[key] = a
	}

	a.count++
	if a.count >= maxFailedAttempts {
		a.lockedUntil = now.Add(attemptLockoutDuration)
	}
}

// recordSuccess clears any tracked failures for key.
func (l *attemptLimiter) recordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
