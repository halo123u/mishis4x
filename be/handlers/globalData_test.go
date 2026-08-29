package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"example.com/mishis4x/api"
	"github.com/stretchr/testify/require"
)

// These prove GetGlobalData's CollectionAccess field actually matches what
// canAccessCollection/ownerOnlyMiddleware would decide for GET /api/sets -
// the frontend hides the Card Manager widget based on this value alone, so
// it needs to agree with what the API itself would actually allow.

func TestGetGlobalData_CollectionAccessTrueForOwner(t *testing.T) {
	db := testDB(t)
	username := testUsername(t, db)
	userID := createTestUser(t, db, username, "correctpass123")

	ts, client := newTestServerWithOwner(t, db, userID)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	res, err := client.Get(ts.URL + "/api/data")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var data api.GlobalData
	require.NoError(t, json.NewDecoder(res.Body).Decode(&data))
	require.True(t, data.CollectionAccess, "the configured owner must see CollectionAccess: true")
}

func TestGetGlobalData_CollectionAccessFalseForNonOwner(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithOwner(t, db, -1)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/data")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var data api.GlobalData
	require.NoError(t, json.NewDecoder(res.Body).Decode(&data))
	require.False(t, data.CollectionAccess, "a real logged-in user who isn't the owner must see CollectionAccess: false, not just 403 on /api/sets")
}

func TestGetGlobalData_CollectionAccessTrueWithAllowAllUsers(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerAllowAllUsers(t, db)
	createAndLoginTestUser(t, db, client, ts.URL)

	res, err := client.Get(ts.URL + "/api/data")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var data api.GlobalData
	require.NoError(t, json.NewDecoder(res.Body).Decode(&data))
	require.True(t, data.CollectionAccess, "CollectionAllowAllUsers must flip this to true for any authenticated user")
}
