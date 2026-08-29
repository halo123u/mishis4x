package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"example.com/mishis4x/api"
	"example.com/mishis4x/persist"
	"github.com/stretchr/testify/require"
)

// createAndLoginTestUser creates a user and logs the client in through the
// real login endpoint (not a direct DB insert of a session) so these tests
// exercise AuthMiddleware/ownerOnlyMiddleware exactly as a real request
// would. Returns the created user's ID, since ownerOnlyMiddleware tests
// need to know it before deciding who the server's owner is.
func createAndLoginTestUser(t *testing.T, db *sql.DB, client *http.Client, baseURL string) int {
	t.Helper()

	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	res := postJSON(t, client, baseURL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	return userID
}

func TestListSets(t *testing.T) {
	db := testDB(t)
	// newTestServer (not WithOwner) here deliberately - see
	// TestListSets_NotTheOwner for the case this test is contrasted against.
	// We need the user's ID before building the server, so this test builds
	// its own server after creating the user rather than using the shared
	// newTestServer/createAndLoginTestUser pair.
	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	ts, client := newTestServerWithOwner(t, db, userID)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID) })

	res, err = client.Get(ts.URL + "/api/sets")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var sets []api.Set
	require.NoError(t, json.NewDecoder(res.Body).Decode(&sets))

	var found bool
	for _, s := range sets {
		if s.ID == setID {
			found = true
			require.Equal(t, "Brown Dust 2", s.Name)
			require.Equal(t, "pending", s.Status)
		}
	}
	require.True(t, found, "the set just created must appear in the list")
}

func TestListSets_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	res, err := client.Get(ts.URL + "/api/sets")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestListSets_NotTheOwner(t *testing.T) {
	db := testDB(t)

	// Server's owner is some other, unrelated user ID - never a real row,
	// which is exactly the point: an ordinary logged-in user must not pass
	// just by virtue of being authenticated. This is the default
	// (CollectionAllowAllUsers unset/false) - see TestListSets_AllowAllUsers
	// for the explicit opt-out.
	ts, client := newTestServerWithOwner(t, db, -1)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/sets")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode, "logged in but not the configured owner must be 403 by default, not 200")
}

func TestListSets_OwnerUnset(t *testing.T) {
	db := testDB(t)

	// CollectionOwnerUserID unset (0) must fail closed by default - nobody
	// passes, including a real, valid, logged-in user - not fail open.
	ts, client := newTestServer(t, db)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/sets")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestListSets_AllowAllUsers(t *testing.T) {
	db := testDB(t)

	// CollectionAllowAllUsers is the explicit opt-out (see its doc comment)
	// - with it set, an ordinary logged-in user who isn't the configured
	// owner (there isn't one here at all) must pass, not be blocked.
	ts, client := newTestServerAllowAllUsers(t, db)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/sets")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode, "CollectionAllowAllUsers must let any authenticated user through")
}

func TestListCardsForSet(t *testing.T) {
	db := testDB(t)
	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	ts, client := newTestServerWithOwner(t, db, userID)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 2, nil, "pending")
	require.NoError(t, err)
	// cards must go before sets - cards.set_id FKs to sets(id).
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	_, err = p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)
	_, err = p.CreateCard(t.Context(), setID, "Pool Party Angelica", "BRD/W139-003S", "SR 2-star")
	require.NoError(t, err)

	res, err = client.Get(ts.URL + "/api/sets/" + setID + "/cards")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var cards []api.Card
	require.NoError(t, json.NewDecoder(res.Body).Decode(&cards))
	require.Len(t, cards, 2)
	// Both are "S" suffix, so star tier decides order: 003S is 2-star,
	// 001S is 3-star - see persist.TestCardLifecycle for the full ordering
	// behavior this is just confirming survives the HTTP/JSON round-trip.
	require.Equal(t, "BRD/W139-003S", cards[0].Code)
	require.Equal(t, "BRD/W139-001S", cards[1].Code)
}

func TestListCardsForSet_NotTheOwner(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithOwner(t, db, -1)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/sets/anything/cards")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode, "must be blocked before even reaching the set-lookup logic")
}

func TestListCardsForSet_NotFound(t *testing.T) {
	db := testDB(t)
	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	ts, client := newTestServerWithOwner(t, db, userID)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	res, err := client.Get(ts.URL + "/api/sets/does-not-exist/cards")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestListCardsForSet_EmptySet(t *testing.T) {
	db := testDB(t)
	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	ts, client := newTestServerWithOwner(t, db, userID)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Empty Set", 0, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID) })

	res, err = client.Get(ts.URL + "/api/sets/" + setID + "/cards")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode, "a real set with no cards yet is not a 404")

	var cards []api.Card
	require.NoError(t, json.NewDecoder(res.Body).Decode(&cards))
	require.Empty(t, cards)
}

func TestListOwnedSets_StartsEmpty(t *testing.T) {
	db := testDB(t)
	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	ts, client := newTestServerWithOwner(t, db, userID)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	// A set existing in the catalog isn't enough - a fresh user's dashboard
	// starts empty until they onboard something, even though ListSets
	// itself is non-empty.
	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID) })

	res, err = client.Get(ts.URL + "/api/owned-sets")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var sets []api.Set
	require.NoError(t, json.NewDecoder(res.Body).Decode(&sets))
	require.Empty(t, sets)
}

func TestListOwnedSets_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	res, err := client.Get(ts.URL + "/api/owned-sets")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestAddOwnedSet_ThenListedAsOwned(t *testing.T) {
	db := testDB(t)
	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	ts, client := newTestServerWithOwner(t, db, userID)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_sets WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	res = postJSON(t, client, ts.URL+"/api/owned-sets", map[string]string{"set_id": setID})
	require.Equal(t, http.StatusNoContent, res.StatusCode)

	// Onboarding the same set twice must not error or duplicate it.
	res = postJSON(t, client, ts.URL+"/api/owned-sets", map[string]string{"set_id": setID})
	require.Equal(t, http.StatusNoContent, res.StatusCode)

	res, err = client.Get(ts.URL + "/api/owned-sets")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var sets []api.Set
	require.NoError(t, json.NewDecoder(res.Body).Decode(&sets))
	require.Len(t, sets, 1, "onboarding twice must not duplicate the set in the list")
	require.Equal(t, setID, sets[0].ID)
}

func TestAddOwnedSet_UnknownSetIsNotFound(t *testing.T) {
	db := testDB(t)
	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	ts, client := newTestServerWithOwner(t, db, userID)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	res = postJSON(t, client, ts.URL+"/api/owned-sets", map[string]string{"set_id": "does-not-exist"})
	require.Equal(t, http.StatusNotFound, res.StatusCode, "must not silently onboard a garbage set_id")
}

func TestAddOwnedSet_NotTheOwner(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithOwner(t, db, -1)
	createAndLoginTestUser(t, db, client, ts.URL)

	res := postJSON(t, client, ts.URL+"/api/owned-sets", map[string]string{"set_id": "anything"})
	require.Equal(t, http.StatusForbidden, res.StatusCode, "must be blocked before even reaching the set-lookup logic")
}

// postSetOwnedCards POSTs input to /api/owned-sets/{setID}/cards. Not built
// on the shared postJSON helper - that one's body type is map[string]string,
// which can't express input's nested Cards slice.
func postSetOwnedCards(t *testing.T, client *http.Client, baseURL, setID string, input api.SetOwnedCardsInput) *http.Response {
	t.Helper()

	b, err := json.Marshal(input)
	require.NoError(t, err)

	res, err := client.Post(baseURL+"/api/owned-sets/"+setID+"/cards", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })

	return res
}

func TestListOwnedCardsForSet(t *testing.T) {
	db := testDB(t)
	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	ts, client := newTestServerWithOwner(t, db, userID)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_cards WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)

	// Nothing owned yet - must be an empty list, not an error.
	res, err = client.Get(ts.URL + "/api/owned-sets/" + setID + "/cards")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var owned []api.OwnedCardInput
	require.NoError(t, json.NewDecoder(res.Body).Decode(&owned))
	require.Empty(t, owned)

	require.NoError(t, p.SetOwnedCards(t.Context(), userID, []persist.CardQuantity{{CardID: cardID, Quantity: 4}}))

	res, err = client.Get(ts.URL + "/api/owned-sets/" + setID + "/cards")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&owned))
	require.Equal(t, []api.OwnedCardInput{{CardID: cardID, Quantity: 4}}, owned)
}

func TestListOwnedCardsForSet_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	res, err := client.Get(ts.URL + "/api/owned-sets/anything/cards")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestListOwnedCardsForSet_NotTheOwner(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithOwner(t, db, -1)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/owned-sets/anything/cards")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestSetOwnedCardsForSet_RecordsOwnership(t *testing.T) {
	db := testDB(t)
	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	ts, client := newTestServerWithOwner(t, db, userID)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 2, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM owned_cards WHERE user_id = ?", userID)
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardOne, err := p.CreateCard(t.Context(), setID, "Poolside Fairy Refithea", "BRD/W139-001S", "SR 3-star")
	require.NoError(t, err)
	cardTwo, err := p.CreateCard(t.Context(), setID, "Michaela", "BRD/W139-009S", "SR 1-star")
	require.NoError(t, err)

	res = postSetOwnedCards(t, client, ts.URL, setID, api.SetOwnedCardsInput{
		Cards: []api.OwnedCardInput{
			{CardID: cardOne, Quantity: 2},
			{CardID: cardTwo, Quantity: 1},
		},
	})
	require.Equal(t, http.StatusNoContent, res.StatusCode)

	ocOne, err := p.GetOwnedCard(t.Context(), userID, cardOne)
	require.NoError(t, err)
	require.Equal(t, 2, ocOne.Quantity)
	ocTwo, err := p.GetOwnedCard(t.Context(), userID, cardTwo)
	require.NoError(t, err)
	require.Equal(t, 1, ocTwo.Quantity)
}

func TestSetOwnedCardsForSet_UnknownCardIsRejected(t *testing.T) {
	db := testDB(t)
	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	ts, client := newTestServerWithOwner(t, db, userID)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	p := &persist.Persist{DB: db}
	setID, err := p.CreateSet(t.Context(), "Brown Dust 2", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	res = postSetOwnedCards(t, client, ts.URL, setID, api.SetOwnedCardsInput{
		Cards: []api.OwnedCardInput{{CardID: "does-not-exist", Quantity: 1}},
	})
	require.Equal(t, http.StatusBadRequest, res.StatusCode, "must not silently attribute ownership of a card that isn't in this set")
}

func TestSetOwnedCardsForSet_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServer(t, db)

	res := postSetOwnedCards(t, client, ts.URL, "anything", api.SetOwnedCardsInput{})
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestSetOwnedCardsForSet_NotTheOwner(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithOwner(t, db, -1)
	createAndLoginTestUser(t, db, client, ts.URL)

	res := postSetOwnedCards(t, client, ts.URL, "anything", api.SetOwnedCardsInput{})
	require.Equal(t, http.StatusForbidden, res.StatusCode, "must be blocked before even reaching the card-validation logic")
}
