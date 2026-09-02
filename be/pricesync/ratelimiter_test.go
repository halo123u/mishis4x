package pricesync

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRateLimiter_TryAcquire(t *testing.T) {
	t.Run("allows up to capacity, then denies", func(t *testing.T) {
		r := NewRateLimiter(3, time.Hour) // refill irrelevant within this test's runtime

		require.True(t, r.TryAcquire())
		require.True(t, r.TryAcquire())
		require.True(t, r.TryAcquire())
		require.False(t, r.TryAcquire(), "the 4th call within capacity=3 must be denied")
	})

	t.Run("refills over time", func(t *testing.T) {
		r := NewRateLimiter(1, 20*time.Millisecond)

		require.True(t, r.TryAcquire())
		require.False(t, r.TryAcquire(), "no tokens left immediately after draining capacity=1")

		time.Sleep(30 * time.Millisecond)
		require.True(t, r.TryAcquire(), "a token should have refilled after waiting past the refill interval")
	})

	t.Run("never refills past capacity", func(t *testing.T) {
		r := NewRateLimiter(2, time.Millisecond)

		time.Sleep(50 * time.Millisecond) // many refill intervals worth of idle time
		require.True(t, r.TryAcquire())
		require.True(t, r.TryAcquire())
		require.False(t, r.TryAcquire(), "capacity=2 must cap accumulated tokens, however long it sat idle")
	})
}
