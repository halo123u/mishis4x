package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"example.com/mishis4x/api"
	"github.com/stretchr/testify/require"
)

func TestGetModelDisplay_DefaultsToEmptyState(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	res, err := client.Get(ts.URL + "/api/models/display")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var state api.ModelDisplayState
	require.NoError(t, json.NewDecoder(res.Body).Decode(&state))
	require.Equal(t, api.ModelDisplayState{}, state, "a server that's never had SetModelDisplay called must report the zero value, not an error")
}

func TestSetModelDisplay_StoresAndGetReflectsIt(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	body, err := json.Marshal(api.ModelDisplayState{CharCode: charCode, Flip: "x", Pepper: true})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/models/display", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	putRes, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = putRes.Body.Close() }()
	require.Equal(t, http.StatusOK, putRes.StatusCode)

	getRes, err := client.Get(ts.URL + "/api/models/display")
	require.NoError(t, err)
	defer func() { _ = getRes.Body.Close() }()
	var state api.ModelDisplayState
	require.NoError(t, json.NewDecoder(getRes.Body).Decode(&state))
	require.Equal(t, api.ModelDisplayState{CharCode: charCode, Flip: "x", Pepper: true}, state)
}

func TestSetModelDisplay_EmptyCharCodeClearsDisplay(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	// Set a real character first, then clear it - an empty CharCode is a
	// deliberate "nothing selected" state, not an error, same as clearing
	// a card's character-model link via SetCardCharacterModel(nil).
	setDisplay := func(t *testing.T, state api.ModelDisplayState) *http.Response {
		t.Helper()
		body, err := json.Marshal(state)
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/models/display", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		require.NoError(t, err)
		return res
	}

	res := setDisplay(t, api.ModelDisplayState{CharCode: charCode})
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()

	res = setDisplay(t, api.ModelDisplayState{})
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()

	getRes, err := client.Get(ts.URL + "/api/models/display")
	require.NoError(t, err)
	defer func() { _ = getRes.Body.Close() }()
	var state api.ModelDisplayState
	require.NoError(t, json.NewDecoder(getRes.Body).Decode(&state))
	require.Equal(t, api.ModelDisplayState{}, state, "clearing must actually clear, not leave the old char_code in place")
}

func TestSetModelDisplay_UnknownCharCodeNotFound(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	body, err := json.Marshal(api.ModelDisplayState{CharCode: "does-not-exist"})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/models/display", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestGetModelDisplay_NonOwnerForbidden(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithModelViewer(t, db)

	username := testUsername(t, db)
	createTestUser(t, db, username, "correctpass123")
	client := newClient(t)
	loginRes := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, loginRes.StatusCode)

	res, err := client.Get(ts.URL + "/api/models/display")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestGetModelDisplay_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithModelViewer(t, db)

	res, err := http.Get(ts.URL + "/api/models/display")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}
