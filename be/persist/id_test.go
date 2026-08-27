package persist

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewUUIDv7_SortsByCreationOrder(t *testing.T) {
	first, err := NewUUIDv7()
	require.NoError(t, err)

	// UUIDv7's timestamp component is millisecond-resolution - sleep past a
	// tick so two IDs generated back-to-back aren't in the same instant,
	// which would make the ordering check meaningless.
	time.Sleep(2 * time.Millisecond)

	second, err := NewUUIDv7()
	require.NoError(t, err)

	require.NotEqual(t, first, second, "two calls must not collide")
	require.Less(t, first, second, "a later UUIDv7 must sort lexically after an earlier one")
}

func TestNewUUIDv7_Format(t *testing.T) {
	id, err := NewUUIDv7()
	require.NoError(t, err)
	require.Len(t, id, 36, "canonical UUID string form is 36 chars (32 hex + 4 hyphens)")
	require.Equal(t, byte('7'), id[14], "version nibble must be 7")
}
