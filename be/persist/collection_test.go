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
	// cards must go before sets - cards.set_id FKs to sets(id), so deleting
	// the set first silently fails (err discarded here on purpose, same as
	// every other cleanup in this file) and leaks both rows.
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	// Created out of both code order and star order, on purpose - none of
	// insertion order, code order alone, or rarity's literal string value
	// should determine what ListCardsBySet returns.
	_, err = p.CreateCard(t.Context(), setID, "Pool Party Angelica", "BRD/W139-003S", "SR 2-star")
	require.NoError(t, err)
	_, err = p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)
	_, err = p.CreateCard(t.Context(), setID, "Eustia/Justia (Signed)", "BRD/W139-002SP", "SP")
	require.NoError(t, err)
	_, err = p.CreateCard(t.Context(), setID, "Pool Party Eustia/Justia", "BRD/W139-002S", "SR 1-star")
	require.NoError(t, err)

	cards, err := p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Len(t, cards, 4)

	// All "S"-suffix cards come before the "SP" one, regardless of number -
	// and within the "S" group, star tier orders them (1-star, then
	// 2-star, then 3-star) ahead of their numeric code.
	require.Equal(t, "BRD/W139-002S", cards[0].Code, "1-star sorts first within the S group")
	require.Equal(t, "BRD/W139-003S", cards[1].Code, "2-star sorts second within the S group")
	require.Equal(t, "BRD/W139-001S", cards[2].Code, "3-star sorts last within the S group, despite being the lowest number")
	require.Equal(t, "BRD/W139-002SP", cards[3].Code, "SP suffix group comes after every S card")
}

func TestCard_UniquePerSetAndCode(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	_, err = p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	_, err = p.CreateCard(t.Context(), setID, "Duplicate Number", "BRD/W139-001S", "SR 3-star")
	require.Error(t, err, "same set_id + code must violate the unique key")
}

func TestGetSetIDByName(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	_, err := p.getSetIDByName(t.Context(), "getSetIDByName Does Not Exist")
	require.ErrorIs(t, err, ErrSetNotFound)

	name := "getSetIDByName Test Set"
	id, err := p.CreateSet(t.Context(), name, 0, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM sets WHERE id = ?", id) })

	found, err := p.getSetIDByName(t.Context(), name)
	require.NoError(t, err)
	require.Equal(t, id, found)
}

func TestGetOrCreateSetByName(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	name := "GetOrCreateSetByName Test Set"
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM sets WHERE name = ?", name) })

	firstID, err := p.GetOrCreateSetByName(t.Context(), name)
	require.NoError(t, err)
	require.NotEmpty(t, firstID)

	// Calling it again for the same name must return the same set, not
	// create a second row - this is the whole point for an importer that
	// re-resolves set_name on every CSV row.
	secondID, err := p.GetOrCreateSetByName(t.Context(), name)
	require.NoError(t, err)
	require.Equal(t, firstID, secondID)

	fetched, err := p.GetSet(t.Context(), firstID)
	require.NoError(t, err)
	require.Equal(t, name, fetched.Name)
	require.Equal(t, "pending", fetched.Status, "auto-created sets get a placeholder status")
}

func TestUpsertSetMetadata_Creates(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	name := "UpsertSetMetadata Create Test Set"
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM sets WHERE name = ?", name) })

	release := time.Date(2026, time.July, 3, 0, 0, 0, 0, time.UTC)
	id, err := p.UpsertSetMetadata(t.Context(), name, 100, &release, "active")
	require.NoError(t, err)
	require.NotEmpty(t, id)

	fetched, err := p.GetSet(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, 100, fetched.CardCount)
	require.Equal(t, "active", fetched.Status)
	require.NotNil(t, fetched.ReleaseDate)
	require.True(t, release.Equal(*fetched.ReleaseDate))
}

func TestUpsertSetMetadata_UpdatesExisting(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	// Starts as a placeholder, the same way GetOrCreateSetByName would
	// leave it - process-set's --set-file is meant to correct exactly this.
	name := "UpsertSetMetadata Update Test Set"
	id, err := p.CreateSet(t.Context(), name, 0, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM sets WHERE id = ?", id) })

	release := time.Date(2026, time.July, 3, 0, 0, 0, 0, time.UTC)
	updatedID, err := p.UpsertSetMetadata(t.Context(), name, 100, &release, "active")
	require.NoError(t, err)
	require.Equal(t, id, updatedID, "must update the existing row, not create a new one")

	fetched, err := p.GetSet(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, 100, fetched.CardCount)
	require.Equal(t, "active", fetched.Status)
	require.NotNil(t, fetched.ReleaseDate)
	require.True(t, release.Equal(*fetched.ReleaseDate))
}

func TestUpsertCard(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	setID, err := p.CreateSet(t.Context(), "UpsertCard Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	code := "BRD/W139-999S"
	require.NoError(t, p.UpsertCard(t.Context(), setID, "Original Name", code, "SR 1-star"))

	cards, err := p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	firstCardID := cards[0].ID

	// Re-running against the same (set_id, code) - the exact scenario of
	// re-importing an updated CSV - must update in place, not insert a
	// second row, and must leave the original id untouched.
	require.NoError(t, p.UpsertCard(t.Context(), setID, "Corrected Name", code, "SR 2-star"))

	cards, err = p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Len(t, cards, 1, "must update in place, not insert a second row")
	require.Equal(t, firstCardID, cards[0].ID, "existing row's id must be left untouched")
	require.Equal(t, "Corrected Name", cards[0].Name)
	require.Equal(t, "SR 2-star", cards[0].Rarity)
}

func TestDeleteSetCascade_NoSetIsNoop(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	err := p.DeleteSetCascade(t.Context(), "DeleteSetCascade Does Not Exist")
	require.NoError(t, err, "wiping a set that was never created must not error")
}

func TestDeleteSetCascade_DeletesCardsThenSet(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	name := "DeleteSetCascade Test Set"
	setID, err := p.CreateSet(t.Context(), name, 1, nil, "pending")
	require.NoError(t, err)
	// No t.Cleanup needed for the happy path - DeleteSetCascade itself is
	// what's being tested, but register one anyway in case an assertion
	// fails partway through and the delete never runs.
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	_, err = p.CreateCard(t.Context(), setID, "Some Card", "BRD/W139-998S", "SR 1-star")
	require.NoError(t, err)

	require.NoError(t, p.DeleteSetCascade(t.Context(), name))

	_, err = p.GetSet(t.Context(), setID)
	require.ErrorIs(t, err, ErrSetNotFound, "the set row itself must be gone")

	cards, err := p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Empty(t, cards, "the set's cards must be gone too")
}

func TestDeleteSetCascade_FailsIfACardIsOwned(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	name := "DeleteSetCascade Owned Test Set"
	setID, err := p.CreateSet(t.Context(), name, 1, nil, "pending")
	require.NoError(t, err)

	cardID, err := p.CreateCard(t.Context(), setID, "Owned Card", "BRD/W139-997S", "SR 1-star")
	require.NoError(t, err)
	require.NoError(t, p.SetCardQuantity(t.Context(), userID, cardID, 1))

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_cards WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	// Catalog-only tooling built on this must not be able to silently blow
	// away a real user's ownership data just because a card they own
	// happens to belong to the set being deleted.
	err = p.DeleteSetCascade(t.Context(), name)
	require.Error(t, err, "must fail loudly via the FK constraint, not silently orphan owned_cards")
}

func TestDeleteCardsForSet_LeavesSetRowIntact(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	name := "DeleteCardsForSet Test Set"
	setID, err := p.CreateSet(t.Context(), name, 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	_, err = p.CreateCard(t.Context(), setID, "Some Card", "BRD/W139-996S", "SR 1-star")
	require.NoError(t, err)

	require.NoError(t, p.DeleteCardsForSet(t.Context(), setID))

	// This is the actual bug this test guards against: process-set
	// --refresh must be able to clear stale cards without the set's own
	// id churning on every run - a churning id breaks anything already
	// linking to it (a bookmarked /collection/{id} URL, owned_sets rows).
	fetched, err := p.GetSet(t.Context(), setID)
	require.NoError(t, err, "the set row itself must survive - only its cards are wiped")
	require.Equal(t, setID, fetched.ID, "the set's id must not change")

	cards, err := p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Empty(t, cards, "the set's cards must be gone")
}

func TestDeleteCardsForSet_FailsIfACardIsOwned(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	name := "DeleteCardsForSet Owned Test Set"
	setID, err := p.CreateSet(t.Context(), name, 1, nil, "pending")
	require.NoError(t, err)

	cardID, err := p.CreateCard(t.Context(), setID, "Owned Card", "BRD/W139-995S", "SR 1-star")
	require.NoError(t, err)
	require.NoError(t, p.SetCardQuantity(t.Context(), userID, cardID, 1))

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_cards WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	err = p.DeleteCardsForSet(t.Context(), setID)
	require.Error(t, err, "must fail loudly via the FK constraint, not silently orphan owned_cards")
}

func TestSortCardsForDisplay(t *testing.T) {
	// No DB needed - this is pure in-memory logic. Deliberately in a
	// deliberately-scrambled input order to prove the sort, not the DB's
	// own default ordering, produces the result.
	cards := []Card{
		{Code: "003S", Rarity: "SR 2-star"},
		{Code: "077SP", Rarity: "SP"},
		{Code: "001S", Rarity: "SR 3-star"},
		{Code: "T11S", Rarity: "TDP"},
		{Code: "002S", Rarity: "SR 1-star"},
		{Code: "098A", Rarity: "AGR"},
		{Code: "024R", Rarity: "RRR"},
		{Code: "002EX", Rarity: "SEC"},
		{Code: "002SP", Rarity: "SP"},
	}

	sortCardsForDisplay(cards)

	var order []string
	for _, c := range cards {
		order = append(order, c.Code)
	}

	// S group first, ordered by star tier (1, 2, 3) ahead of number, then
	// SP group ordered by number, then rarer chase tiers, then the
	// trial-deck "T"-prefixed code last since it's its own prefix group.
	require.Equal(t, []string{
		"002S",  // S, 1-star
		"003S",  // S, 2-star
		"001S",  // S, 3-star
		"002SP", // SP
		"077SP", // SP
		"002EX", // SEC
		"024R",  // RRR
		"098A",  // AGR
		"T11S",  // different prefix group entirely
	}, order)
}

func TestCardSortKey_UnrecognizedCodeShapeFallsBackToRawString(t *testing.T) {
	// A code that doesn't even end in digits+letters (never seen in
	// practice, but not enforced at the DB level) must not panic or error -
	// it should just sort by its own raw string, after everything
	// recognized.
	prefix, rank, star, num := cardSortKey(Card{Code: "???", Rarity: "unknown"})
	require.Equal(t, "???", prefix)
	require.Equal(t, len(cardSuffixRank)+1, rank)
	require.Equal(t, 0, star)
	require.Equal(t, 0, num)
}
