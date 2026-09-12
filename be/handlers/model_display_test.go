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

// setModelDisplay PUTs a char_code/flip pair to /api/models/display -
// the shape SetModelDisplay actually accepts (api.
// SetModelDisplayCharCodeInput), not the full ModelDisplayState GET
// returns - see SetModelDisplay's own doc comment for why those are two
// different types.
func setModelDisplay(t *testing.T, client *http.Client, tsURL string, input api.SetModelDisplayCharCodeInput) *http.Response {
	t.Helper()
	body, err := json.Marshal(input)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, tsURL+"/api/models/display", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	require.NoError(t, err)
	return res
}

func TestSetModelDisplay_StoresAndGetReflectsIt(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	putRes := setModelDisplay(t, client, ts.URL, api.SetModelDisplayCharCodeInput{CharCode: charCode, Flip: "x"})
	defer func() { _ = putRes.Body.Close() }()
	require.Equal(t, http.StatusOK, putRes.StatusCode)

	state := getModelDisplay(t, client, ts.URL)
	require.Equal(t, api.ModelDisplayState{CharCode: charCode, Flip: "x"}, state)
}

func TestSetModelDisplay_EmptyCharCodeClearsDisplay(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	// Set a real character first, then clear it - an empty CharCode is a
	// deliberate "nothing selected" state, not an error, same as clearing
	// a card's character-model link via SetCardCharacterModel(nil).
	res := setModelDisplay(t, client, ts.URL, api.SetModelDisplayCharCodeInput{CharCode: charCode})
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()

	res = setModelDisplay(t, client, ts.URL, api.SetModelDisplayCharCodeInput{})
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()

	state := getModelDisplay(t, client, ts.URL)
	require.Equal(t, api.ModelDisplayState{}, state, "clearing must actually clear, not leave the old char_code in place")
}

func TestSetModelDisplay_UnknownCharCodeNotFound(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	res := setModelDisplay(t, client, ts.URL, api.SetModelDisplayCharCodeInput{CharCode: "does-not-exist"})
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

	putRes := setModelDisplay(t, client, ts.URL, api.SetModelDisplayCharCodeInput{CharCode: charCode})
	require.Equal(t, http.StatusOK, putRes.StatusCode)
	_ = putRes.Body.Close()

	time.Sleep(20 * time.Millisecond)

	status := getModelDisplayStatus(t, client, ts.URL)
	require.False(t, status.Connected)

	state := getModelDisplay(t, client, ts.URL)
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

	// "Set as display" only ever sends char_code/flip - it must not
	// silently reset Trigger back to its zero value, which would
	// otherwise register as a real change and fire a spurious touch
	// reaction on the display's very next poll.
	putRes := setModelDisplay(t, client, ts.URL, api.SetModelDisplayCharCodeInput{CharCode: charCode})
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

// setModelDisplayFlip PUTs to /api/models/display/flip - the live-tweak
// endpoint that touches only Flip, separate from setModelDisplay above -
// see SetModelDisplayFlip's own doc comment.
func setModelDisplayFlip(t *testing.T, client *http.Client, tsURL string, flip string) *http.Response {
	t.Helper()
	body, err := json.Marshal(api.SetModelDisplayFlipInput{Flip: flip})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, tsURL+"/api/models/display/flip", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	require.NoError(t, err)
	return res
}

func TestSetModelDisplayFlip_UpdatesFlipOnly(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	putRes := setModelDisplay(t, client, ts.URL, api.SetModelDisplayCharCodeInput{CharCode: charCode, Flip: "x"})
	require.Equal(t, http.StatusOK, putRes.StatusCode)
	_ = putRes.Body.Close()

	flipRes := setModelDisplayFlip(t, client, ts.URL, "y")
	defer func() { _ = flipRes.Body.Close() }()
	require.Equal(t, http.StatusOK, flipRes.StatusCode)

	state := getModelDisplay(t, client, ts.URL)
	require.Equal(t, charCode, state.CharCode, "SetModelDisplayFlip must not touch CharCode")
	require.Equal(t, "y", state.Flip)
}

func TestSetModelDisplayFlip_DoesNotResetTrigger(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	res := triggerModelDisplay(t, client, ts.URL)
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()
	require.Equal(t, 1, getModelDisplay(t, client, ts.URL).Trigger)

	flipRes := setModelDisplayFlip(t, client, ts.URL, "180")
	require.Equal(t, http.StatusOK, flipRes.StatusCode)
	_ = flipRes.Body.Close()

	state := getModelDisplay(t, client, ts.URL)
	require.Equal(t, "180", state.Flip)
	require.Equal(t, 1, state.Trigger, "SetModelDisplayFlip must not reset Trigger")
}

func TestSetModelDisplayFlip_NonOwnerForbidden(t *testing.T) {
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

	res := setModelDisplayFlip(t, client, ts.URL, "x")
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestSetModelDisplayFlip_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithModelViewer(t, db)

	body, err := json.Marshal(api.SetModelDisplayFlipInput{Flip: "x"})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/models/display/flip", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

// setModelDisplayTransform PUTs to /api/models/display/transform - the
// pan/zoom live-tweak endpoint, separate from both setModelDisplay and
// setModelDisplayFlip above - see SetModelDisplayTransform's own doc
// comment.
func setModelDisplayTransform(t *testing.T, client *http.Client, tsURL string, input api.SetModelDisplayTransformInput) *http.Response {
	t.Helper()
	body, err := json.Marshal(input)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, tsURL+"/api/models/display/transform", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	require.NoError(t, err)
	return res
}

func TestSetModelDisplayTransform_UpdatesOffsetAndZoomOnly(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	putRes := setModelDisplay(t, client, ts.URL, api.SetModelDisplayCharCodeInput{CharCode: charCode, Flip: "x"})
	require.Equal(t, http.StatusOK, putRes.StatusCode)
	_ = putRes.Body.Close()

	transformRes := setModelDisplayTransform(t, client, ts.URL, api.SetModelDisplayTransformInput{OffsetX: 20, OffsetY: -10, Zoom: 1.5})
	defer func() { _ = transformRes.Body.Close() }()
	require.Equal(t, http.StatusOK, transformRes.StatusCode)

	state := getModelDisplay(t, client, ts.URL)
	require.Equal(t, charCode, state.CharCode, "SetModelDisplayTransform must not touch CharCode")
	require.Equal(t, "x", state.Flip, "SetModelDisplayTransform must not touch Flip")
	require.Equal(t, 20, state.OffsetX)
	require.Equal(t, -10, state.OffsetY)
	require.InDelta(t, 1.5, state.Zoom, 0.0001)
}

func TestSetModelDisplayTransform_DoesNotResetTrigger(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	res := triggerModelDisplay(t, client, ts.URL)
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()
	require.Equal(t, 1, getModelDisplay(t, client, ts.URL).Trigger)

	transformRes := setModelDisplayTransform(t, client, ts.URL, api.SetModelDisplayTransformInput{OffsetX: 5, OffsetY: 5, Zoom: 2})
	require.Equal(t, http.StatusOK, transformRes.StatusCode)
	_ = transformRes.Body.Close()

	state := getModelDisplay(t, client, ts.URL)
	require.Equal(t, 5, state.OffsetX)
	require.Equal(t, 1, state.Trigger, "SetModelDisplayTransform must not reset Trigger")
}

func TestSetModelDisplayTransform_NonOwnerForbidden(t *testing.T) {
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

	res := setModelDisplayTransform(t, client, ts.URL, api.SetModelDisplayTransformInput{OffsetX: 1})
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestSetModelDisplayTransform_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithModelViewer(t, db)

	body, err := json.Marshal(api.SetModelDisplayTransformInput{OffsetX: 1})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/models/display/transform", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}
