package handlers

import (
	"sync"
	"time"
)

const (
	maxFailedLoginAttempts = 5
	loginLockoutWindow     = 15 * time.Minute
	loginLockoutDuration   = 5 * time.Minute
)

type loginAttempt struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

func (a *loginAttempt) expired(now time.Time) bool {
	return now.Sub(a.windowStart) > loginLockoutWindow && now.After(a.lockedUntil)
}

// loginLimiter tracks failed login attempts per username, in memory, to
// slow down credential guessing against one account. This app runs as a
// single instance (the same constraint documented for matchmaking's
// in-memory state in CLAUDE.md applies here), so in-memory is a deliberate
// choice: no DB round-trip on every login attempt, no cleanup job to run -
// at the cost of resetting on restart, which is an acceptable tradeoff for
// this threat model.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]*loginAttempt)}
}

// locked reports whether username is currently locked out.
func (l *loginLimiter) locked(username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[username]
	if !ok {
		return false
	}
	return time.Now().Before(a.lockedUntil)
}

// recordFailure records a failed attempt for username, locking it out once
// it crosses maxFailedLoginAttempts within loginLockoutWindow. Also sweeps
// fully-expired entries out of the map so it doesn't grow unbounded.
func (l *loginLimiter) recordFailure(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	for u, a := range l.attempts {
		if a.expired(now) {
			delete(l.attempts, u)
		}
	}

	a, ok := l.attempts[username]
	if !ok || now.Sub(a.windowStart) > loginLockoutWindow {
		a = &loginAttempt{windowStart: now}
		l.attempts[username] = a
	}

	a.count++
	if a.count >= maxFailedLoginAttempts {
		a.lockedUntil = now.Add(loginLockoutDuration)
	}
}

// recordSuccess clears any tracked failures for username.
func (l *loginLimiter) recordSuccess(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, username)
}
