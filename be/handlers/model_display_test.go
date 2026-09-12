package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"example.com/mishis4x/api"
	"github.com/stretchr/testify/require"
)

// setDisplayStaleAfterForTest points displayStaleAfter at a tiny
// duration for the lifetime of one test, restoring the real value on
// cleanup - see that var's own doc comment for why it's a var at all.
func setDisplayStaleAfterForTest(t *testing.T, d time.Duration) {
	t.Helper()
	original := displayStaleAfter
	displayStaleAfter = d
	t.Cleanup(func() { displayStaleAfter = original })
}

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

func getModelDisplayStatus(t *testing.T, client *http.Client, tsURL string) api.ModelDisplayStatus {
	t.Helper()
	res, err := client.Get(tsURL + "/api/models/display/status")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var status api.ModelDisplayStatus
	require.NoError(t, json.NewDecoder(res.Body).Decode(&status))
	return status
}

func TestGetModelDisplayStatus_NeverPolledIsDisconnected(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	status := getModelDisplayStatus(t, client, ts.URL)
	require.False(t, status.Connected, "a server that's never had GET /api/models/display called must report disconnected, not an error")
}

func TestGetModelDisplayStatus_RecentlyPolledIsConnected(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	// Simulates the display's own poll - this is the call that's
	// supposed to count as a heartbeat, unlike the status check itself.
	res, err := client.Get(ts.URL + "/api/models/display")
	require.NoError(t, err)
	_ = res.Body.Close()

	status := getModelDisplayStatus(t, client, ts.URL)
	require.True(t, status.Connected)
}

func TestGetModelDisplayStatus_CheckingStatusIsNotItselfAHeartbeat(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	setDisplayStaleAfterForTest(t, 10*time.Millisecond)

	// One real poll, then only status checks from here - if a status
	// check itself counted as proof of life, this would stay "connected"
	// forever regardless of the shrunk threshold, defeating the whole
	// point of Connected being a separate, non-mutating read.
	res, err := client.Get(ts.URL + "/api/models/display")
	require.NoError(t, err)
	_ = res.Body.Close()

	time.Sleep(20 * time.Millisecond)

	status := getModelDisplayStatus(t, client, ts.URL)
	require.False(t, status.Connected, "checking status repeatedly must not itself keep the display looking alive")
}

func TestGetModelDisplayStatus_StaleClearsStoredState(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)
	setDisplayStaleAfterForTest(t, 10*time.Millisecond)

	pollRes, err := client.Get(ts.URL + "/api/models/display")
	require.NoError(t, err)
	_ = pollRes.Body.Close()

	body, err := json.Marshal(api.ModelDisplayState{CharCode: charCode})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/models/display", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	putRes, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, putRes.StatusCode)
	_ = putRes.Body.Close()

	time.Sleep(20 * time.Millisecond)

	status := getModelDisplayStatus(t, client, ts.URL)
	require.False(t, status.Connected)

	getRes, err := client.Get(ts.URL + "/api/models/display")
	require.NoError(t, err)
	defer func() { _ = getRes.Body.Close() }()
	var state api.ModelDisplayState
	require.NoError(t, json.NewDecoder(getRes.Body).Decode(&state))
	require.Equal(t, api.ModelDisplayState{}, state, "a stale display's stored character must actually be cleared, not just reported disconnected")
}

func getModelDisplay(t *testing.T, client *http.Client, tsURL string) api.ModelDisplayState {
	t.Helper()
	res, err := client.Get(tsURL + "/api/models/display")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var state api.ModelDisplayState
	require.NoError(t, json.NewDecoder(res.Body).Decode(&state))
	return state
}

func triggerModelDisplay(t *testing.T, client *http.Client, tsURL string) *http.Response {
	t.Helper()
	res, err := client.Post(tsURL+"/api/models/display/trigger", "", nil)
	require.NoError(t, err)
	return res
}

func TestTriggerModelDisplay_IncrementsCounter(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	require.Equal(t, 0, getModelDisplay(t, client, ts.URL).Trigger)

	res := triggerModelDisplay(t, client, ts.URL)
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()
	require.Equal(t, 1, getModelDisplay(t, client, ts.URL).Trigger)

	res = triggerModelDisplay(t, client, ts.URL)
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()
	require.Equal(t, 2, getModelDisplay(t, client, ts.URL).Trigger)
}

func TestSetModelDisplay_DoesNotResetTrigger(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	res := triggerModelDisplay(t, client, ts.URL)
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()
	require.Equal(t, 1, getModelDisplay(t, client, ts.URL).Trigger)

	// "Set as display" only ever sends char_code/flip/pepper - it must
	// not silently reset Trigger back to its zero value, which would
	// otherwise register as a real change and fire a spurious touch
	// reaction on the display's very next poll.
	body, err := json.Marshal(api.ModelDisplayState{CharCode: charCode})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/models/display", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	putRes, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, putRes.StatusCode)
	_ = putRes.Body.Close()

	state := getModelDisplay(t, client, ts.URL)
	require.Equal(t, charCode, state.CharCode)
	require.Equal(t, 1, state.Trigger, "SetModelDisplay must not reset Trigger")
}

func TestTriggerModelDisplay_NonOwnerForbidden(t *testing.T) {
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

	res := triggerModelDisplay(t, client, ts.URL)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestTriggerModelDisplay_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithModelViewer(t, db)

	res, err := http.Post(ts.URL+"/api/models/display/trigger", "", nil)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}
