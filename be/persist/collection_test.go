package persist

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSetLifecycle(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	release := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	id, err := p.CreateSet(t.Context(), "Brown Dust 2", 100, &release, "pending")
	require.NoError(t, err)
	require.NotEmpty(t, id)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM sets WHERE id = ?", id) })

	fetched, err := p.GetSet(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, id, fetched.ID)
	require.Equal(t, "Brown Dust 2", fetched.Name)
	require.Equal(t, 100, fetched.CardCount)
	require.Equal(t, "pending", fetched.Status)
	require.NotNil(t, fetched.ReleaseDate)
	require.True(t, release.Equal(*fetched.ReleaseDate))
}

func TestSet_NotFound(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	_, err := p.GetSet(t.Context(), "does-not-exist")
	require.ErrorIs(t, err, ErrSetNotFound)
}

func TestSet_NilReleaseDate(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	id, err := p.CreateSet(t.Context(), "TBD Set", 0, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM sets WHERE id = ?", id) })

	fetched, err := p.GetSet(t.Context(), id)
	require.NoError(t, err)
	require.Nil(t, fetched.ReleaseDate)
}

func TestCardLifecycle(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 2, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID) })

	firstID, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	secondID, err := p.CreateCard(t.Context(), setID, "Pool Party Angelica", "BRD/W139-003S", "SR 2-star")
	require.NoError(t, err)

	cards, err := p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Len(t, cards, 2)

	// ListCardsBySet orders by id (UUIDv7), which should match insertion
	// order - this is the whole point of using a sortable ID scheme.
	require.Equal(t, firstID, cards[0].ID)
	require.Equal(t, secondID, cards[1].ID)
	require.Equal(t, "BRD/W139-001S", cards[0].Code)
	require.Equal(t, "SR 3-star", cards[0].Rarity)
}

func TestCard_UniquePerSetAndCode(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID) })

	_, err = p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	_, err = p.CreateCard(t.Context(), setID, "Duplicate Number", "BRD/W139-001S", "SR 3-star")
	require.Error(t, err, "same set_id + code must violate the unique key")
}
