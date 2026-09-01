package persist

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCardImage_NoneStoredReturnsNotFound(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	_, _, _, err = p.GetCardImage(t.Context(), cardID)
	require.ErrorIs(t, err, ErrCardImageNotFound, "a card with no image yet must not be an error, but must be distinguishable from a real one")
}

func TestUpsertCardImage_StoreAndUpdate(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	// card_images isn't cleaned up separately - it cascades automatically
	// when its card is deleted (see the migration's ON DELETE CASCADE).
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	original := []byte("not a real jpeg, just test bytes")
	require.NoError(t, p.UpsertCardImage(t.Context(), cardID, original, "image/jpeg"))

	image, contentType, firstUpdatedAt, err := p.GetCardImage(t.Context(), cardID)
	require.NoError(t, err)
	require.Equal(t, original, image)
	require.Equal(t, "image/jpeg", contentType)
	require.False(t, firstUpdatedAt.IsZero())

	// Re-running (e.g. re-processing the same set with --images-dir)
	// must replace in place, not error or leave the old bytes.
	updated := []byte("a different, updated image")
	require.NoError(t, p.UpsertCardImage(t.Context(), cardID, updated, "image/png"))

	image, contentType, secondUpdatedAt, err := p.GetCardImage(t.Context(), cardID)
	require.NoError(t, err)
	require.Equal(t, updated, image)
	require.Equal(t, "image/png", contentType)
	// Not a strict "after" check - updated_at has only second granularity,
	// so two upserts in the same test can legitimately land on the same
	// timestamp. What matters here is that it's never *older*, which
	// would mean ON UPDATE CURRENT_TIMESTAMP silently isn't firing - the
	// exact thing GetCardImage's Last-Modified/conditional-request
	// support (handlers.Data.GetCardImage) depends on.
	require.False(t, secondUpdatedAt.Before(firstUpdatedAt))
}

func TestCardImage_CascadesOnCardDelete(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	require.NoError(t, p.UpsertCardImage(t.Context(), cardID, []byte("test image"), "image/jpeg"))

	// Unlike owned_cards (which deliberately blocks this - see
	// TestDeleteCardsForSet_FailsIfACardIsOwned), a stored image must not
	// block deleting the card, and must vanish along with it via the FK's
	// ON DELETE CASCADE, not linger as an orphaned row.
	require.NoError(t, p.DeleteCardsForSet(t.Context(), setID))

	_, _, _, err = p.GetCardImage(t.Context(), cardID)
	require.ErrorIs(t, err, ErrCardImageNotFound, "the image must be gone once its card is, not orphaned")
}
