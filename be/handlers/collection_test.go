package handlers

import (
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
	// just by virtue of being authenticated.
	ts, client := newTestServerWithOwner(t, db, -1)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/sets")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode, "logged in but not the configured owner must be 403, not 200")
}

func TestListSets_OwnerUnset(t *testing.T) {
	db := testDB(t)

	// CollectionOwnerUserID unset (0) must fail closed - nobody passes,
	// including a real, valid, logged-in user - not fail open.
	ts, client := newTestServer(t, db)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/sets")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
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
	require.Equal(t, "BRD/W139-001S", cards[0].Code)
	require.Equal(t, "BRD/W139-003S", cards[1].Code)
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
