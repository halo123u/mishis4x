package persist

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// setupOwnershipTestUser creates a real user row, since owned_sets/
// owned_cards FK to users(id) - can't test ownership without one.
func setupOwnershipTestUser(t *testing.T, p *Persist) int {
	t.Helper()

	username := fmt.Sprintf("ownership-test-user-%d-%s", os.Getpid(), t.Name())
	t.Cleanup(func() { _, _ = p.DB.Exec("DELETE FROM users WHERE username = ?", username) })

	userID, err := p.CreateUser(t.Context(), User{Username: username, Status: "active", Password: "hashedpw"})
	require.NoError(t, err)

	return userID
}

func TestOwnedSetLifecycle(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	// owned_sets before sets - owned_sets.set_id FKs to sets(id).
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_sets WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	require.NoError(t, p.SetOwnedSet(t.Context(), userID, setID))

	setIDs, err := p.ListOwnedSetIDs(t.Context(), userID)
	require.NoError(t, err)
	require.Equal(t, []string{setID}, setIDs)
}

func TestSetOwnedSet_Idempotent(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_sets WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	require.NoError(t, p.SetOwnedSet(t.Context(), userID, setID))
	require.NoError(t, p.SetOwnedSet(t.Context(), userID, setID), "onboarding the same set twice must not error")

	setIDs, err := p.ListOwnedSetIDs(t.Context(), userID)
	require.NoError(t, err)
	require.Len(t, setIDs, 1, "must not duplicate the row")
}

func TestOwnedCard_NotOwnedReturnsZeroQuantity(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	// cards must go before sets - cards.set_id FKs to sets(id).
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	oc, err := p.GetOwnedCard(t.Context(), userID, cardID)
	require.NoError(t, err, "no ownership row yet must not be an error")
	require.Equal(t, 0, oc.Quantity)
}

func TestSetCardQuantity_UpsertAndUpdate(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	// owned_cards, then cards, then sets - each FKs to the previous.
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_card_copies WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	require.NoError(t, p.SetCardQuantity(t.Context(), userID, cardID, 1))
	oc, err := p.GetOwnedCard(t.Context(), userID, cardID)
	require.NoError(t, err)
	require.Equal(t, 1, oc.Quantity)

	// Duplicate purchase - exactly the manual-tracking problem quantity
	// exists to solve (see 047S in the actual collecting session).
	require.NoError(t, p.SetCardQuantity(t.Context(), userID, cardID, 2))
	oc, err = p.GetOwnedCard(t.Context(), userID, cardID)
	require.NoError(t, err)
	require.Equal(t, 2, oc.Quantity, "must update in place, not insert a second row")
}

func TestDeleteOwnedSet_RemovesSetAndItsCards(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_card_copies WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM owned_sets WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	require.NoError(t, p.SetOwnedSet(t.Context(), userID, setID))
	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{{CardID: cardID, Quantity: 2}}))

	require.NoError(t, p.DeleteOwnedSet(t.Context(), userID, setID))

	setIDs, err := p.ListOwnedSetIDs(t.Context(), userID)
	require.NoError(t, err)
	require.Empty(t, setIDs, "the set must no longer be onboarded")

	oc, err := p.GetOwnedCard(t.Context(), userID, cardID)
	require.NoError(t, err)
	require.Equal(t, 0, oc.Quantity, "card ownership must be cleared, not left resurrectable")

	// The underlying catalog card must still exist - only ownership data
	// was removed, not the card itself.
	cards, err := p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Len(t, cards, 1)
}

func TestDeleteOwnedSet_NeverOnboardedIsNoop(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	require.NoError(t, p.DeleteOwnedSet(t.Context(), userID, "does-not-exist"), "deleting a set that was never onboarded must not error")
}

func TestSetOwnedCards_BulkUpsertAndUpdate(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_card_copies WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardOne, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)
	cardTwo, err := p.CreateCard(t.Context(), setID, "Michaela", "BRD/W139-009S", "SR 1-star")
	require.NoError(t, err)

	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{
		{CardID: cardOne, Quantity: 2},
		{CardID: cardTwo, Quantity: 1},
	}))

	ocOne, err := p.GetOwnedCard(t.Context(), userID, cardOne)
	require.NoError(t, err)
	require.Equal(t, 2, ocOne.Quantity)
	ocTwo, err := p.GetOwnedCard(t.Context(), userID, cardTwo)
	require.NoError(t, err)
	require.Equal(t, 1, ocTwo.Quantity)

	// Submitting again with an updated quantity for one card must update
	// in place, not insert a second row or disturb the other card.
	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{
		{CardID: cardOne, Quantity: 3},
	}))
	ocOne, err = p.GetOwnedCard(t.Context(), userID, cardOne)
	require.NoError(t, err)
	require.Equal(t, 3, ocOne.Quantity)
	ocTwo, err = p.GetOwnedCard(t.Context(), userID, cardTwo)
	require.NoError(t, err)
	require.Equal(t, 1, ocTwo.Quantity, "must not touch a card not present in this call")
}

func TestSetOwnedCards_PricePaidCents(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_card_copies WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	// No price given at all - nil, not $0, since the two mean different
	// things (unknown vs. a genuine free acquisition).
	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{
		{CardID: cardID, Quantity: 1},
	}))
	oc, err := p.GetOwnedCard(t.Context(), userID, cardID)
	require.NoError(t, err)
	require.Nil(t, oc.PricePaidCents, "no price submitted must stay nil, not default to 0")

	// Recording a real price - $16.33, matching the actual first entry in
	// the personal tracker this feature is meant to replace.
	priceCents := 1633
	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{
		{CardID: cardID, Quantity: 1, PricePaidCents: &priceCents},
	}))
	oc, err = p.GetOwnedCard(t.Context(), userID, cardID)
	require.NoError(t, err)
	require.NotNil(t, oc.PricePaidCents)
	require.Equal(t, 1633, *oc.PricePaidCents)

	// Submitting nil again fully replaces the stored price back to
	// unknown - price follows the same "whatever's submitted wins" rule
	// quantity already does, it's never merged with what's there.
	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{
		{CardID: cardID, Quantity: 1},
	}))
	oc, err = p.GetOwnedCard(t.Context(), userID, cardID)
	require.NoError(t, err)
	require.Nil(t, oc.PricePaidCents, "resubmitting without a price must clear the old one, not leave it untouched")

	// ListOwnedCardsBySet must surface the same field.
	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{
		{CardID: cardID, Quantity: 1, PricePaidCents: &priceCents},
	}))
	owned, err := p.ListOwnedCardsBySet(t.Context(), userID, setID)
	require.NoError(t, err)
	require.Len(t, owned, 1)
	require.NotNil(t, owned[0].PricePaidCents)
	require.Equal(t, 1633, *owned[0].PricePaidCents)
}

func TestListOwnedCardsBySet(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 2, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_card_copies WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardOne, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)
	cardTwo, err := p.CreateCard(t.Context(), setID, "Michaela", "BRD/W139-009S", "SR 1-star")
	require.NoError(t, err)

	// Nothing interacted with yet - must be empty, not a row per card at
	// quantity 0.
	owned, err := p.ListOwnedCardsBySet(t.Context(), userID, setID)
	require.NoError(t, err)
	require.Empty(t, owned)

	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{
		{CardID: cardOne, Quantity: 2},
		{CardID: cardTwo, Quantity: 0}, // explicitly marked not owned
	}))

	// Post-#108, quantity 0 means zero owned_card_copies rows, which is
	// indistinguishable from a card nobody ever touched - so an explicit
	// 0 no longer appears here at all, unlike the old stored-quantity-
	// column model. See ListOwnedCardsBySet's doc comment.
	owned, err = p.ListOwnedCardsBySet(t.Context(), userID, setID)
	require.NoError(t, err)
	require.Equal(t, []CardQuantity{
		{CardID: cardOne, Quantity: 2},
	}, owned, "a card explicitly set to 0 has no copies left to list, same as one never touched")
}

func TestSetOwnedCards_EmptyIsNoop(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	require.NoError(t, p.SetOwnedCards(t.Context(), userID, nil), "an empty call must not error")
}

func TestListOwnedSets(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	// Nothing onboarded yet - a fresh user's dashboard starts empty even
	// though the catalog itself isn't.
	sets, err := p.ListOwnedSets(t.Context(), userID)
	require.NoError(t, err)
	require.Empty(t, sets)

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_sets WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	// A set existing in the catalog isn't enough on its own - onboarding
	// it is what makes it show up here.
	sets, err = p.ListOwnedSets(t.Context(), userID)
	require.NoError(t, err)
	require.Empty(t, sets, "a real catalog set must not appear until onboarded")

	require.NoError(t, p.SetOwnedSet(t.Context(), userID, setID))

	sets, err = p.ListOwnedSets(t.Context(), userID)
	require.NoError(t, err)
	require.Len(t, sets, 1)
	require.Equal(t, setID, sets[0].ID)
	require.Equal(t, "Brown Dust 2", sets[0].Name)
}

// TestSetOwnedCards_IncrementalPriceOnlyAffectsNewCopy is #108's actual
// point: buying a 2nd copy at a different price than the 1st no longer
// overwrites what the 1st copy was recorded as costing - each owned_card_
// copies row keeps its own price, and GetOwnedCard's PricePaidCents (a
// SUM across a card's copies, see OwnedCard's doc comment) reflects both.
func TestSetOwnedCards_IncrementalPriceOnlyAffectsNewCopy(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_card_copies WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	// First copy, bought for $10.
	firstPrice := 1000
	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{
		{CardID: cardID, Quantity: 1, PricePaidCents: &firstPrice},
	}))

	// A 2nd copy, bought later for $15 - a real duplicate-at-a-different-
	// price purchase, exactly what this feature exists for.
	secondPrice := 1500
	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{
		{CardID: cardID, Quantity: 2, PricePaidCents: &secondPrice},
	}))

	oc, err := p.GetOwnedCard(t.Context(), userID, cardID)
	require.NoError(t, err)
	require.Equal(t, 2, oc.Quantity)
	require.NotNil(t, oc.PricePaidCents)
	require.Equal(t, 2500, *oc.PricePaidCents, "the 1st copy's $10 must survive alongside the 2nd copy's $15, not get overwritten by it")

	var firstCopyPrice sql.NullInt64
	require.NoError(t, db.QueryRow(
		"SELECT price_paid_cents FROM owned_card_copies WHERE user_id = ? AND card_id = ? ORDER BY created_at, id LIMIT 1",
		userID, cardID,
	).Scan(&firstCopyPrice))
	require.True(t, firstCopyPrice.Valid)
	require.Equal(t, int64(1000), firstCopyPrice.Int64, "the original copy's own row must still say $10")
}

// TestSetCardQuantity_DecreaseRemovesMostRecentCopyFirst confirms the LIFO
// policy reconcileOwnedCardCopies documents: dropping quantity removes the
// most-recently-added copy, leaving the earlier one (and its price) alone.
func TestSetCardQuantity_DecreaseRemovesMostRecentCopyFirst(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	userID := setupOwnershipTestUser(t, p)

	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_card_copies WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	firstPrice := 1000
	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{
		{CardID: cardID, Quantity: 1, PricePaidCents: &firstPrice},
	}))
	secondPrice := 1500
	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []CardQuantity{
		{CardID: cardID, Quantity: 2, PricePaidCents: &secondPrice},
	}))

	// Back down to 1 copy - the 2nd (most recent, $15) one should go,
	// leaving the original $10 copy behind.
	require.NoError(t, p.SetCardQuantity(t.Context(), userID, cardID, 1))

	oc, err := p.GetOwnedCard(t.Context(), userID, cardID)
	require.NoError(t, err)
	require.Equal(t, 1, oc.Quantity)
	require.NotNil(t, oc.PricePaidCents)
	require.Equal(t, 1000, *oc.PricePaidCents, "the surviving copy must be the original $10 one, not the $15 one")
}
