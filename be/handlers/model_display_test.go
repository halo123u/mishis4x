package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/mishis4x/api"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// dialModelDisplayWS opens an authenticated WebSocket connection to
// /api/models/display/ws, the same way the real display does - client's
// cookie jar carries the session, exactly like an ordinary authenticated
// HTTP request would (the upgrade request is still a plain HTTP request
// under the hood, gated by the same AuthMiddleware/modelOnlyMiddleware
// chain as everything else under /api/models/...).
func dialModelDisplayWS(t *testing.T, client *http.Client, tsURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(tsURL, "http") + "/api/models/display/ws"
	conn, res, err := (&websocket.Dialer{Jar: client.Jar}).Dial(wsURL, nil)
	require.NoError(t, err)
	if res != nil {
		_ = res.Body.Close()
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
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

func TestGetModelDisplayStatus_NoDisplayIsDisconnected(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	status := getModelDisplayStatus(t, client, ts.URL)
	require.False(t, status.Connected, "a server with no display WebSocket connected must report disconnected, not an error")
}

func TestGetModelDisplayStatus_ConnectedWhileWebSocketOpen(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	dialModelDisplayWS(t, client, ts.URL)

	// Eventually, not an immediate assertion: the client's Dial() call
	// returns as soon as the 101 Switching Protocols response comes
	// back, which can land a moment before ServeModelDisplayWS's own
	// subsequent subscribe() call actually runs server-side - a real,
	// if narrow, race rather than a flaky test artifact.
	require.Eventually(t, func() bool {
		return getModelDisplayStatus(t, client, ts.URL).Connected
	}, time.Second, 10*time.Millisecond, "must report connected once a display's WebSocket handshake completes")
}

func TestGetModelDisplayStatus_DisconnectsAfterWebSocketCloses(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	conn := dialModelDisplayWS(t, client, ts.URL)
	require.Eventually(t, func() bool {
		return getModelDisplayStatus(t, client, ts.URL).Connected
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, conn.Close())

	// Eventually: the server only notices a closed connection once its
	// own blocked ReadMessage call returns an error, which happens
	// promptly but not synchronously with this test's own conn.Close().
	require.Eventually(t, func() bool {
		return !getModelDisplayStatus(t, client, ts.URL).Connected
	}, time.Second, 10*time.Millisecond, "must report disconnected once the display's WebSocket actually closes")
}

func TestModelDisplayWS_DisconnectClearsStoredState(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	conn := dialModelDisplayWS(t, client, ts.URL)
	require.Eventually(t, func() bool {
		return getModelDisplayStatus(t, client, ts.URL).Connected
	}, time.Second, 10*time.Millisecond)

	putRes := setModelDisplay(t, client, ts.URL, api.SetModelDisplayCharCodeInput{CharCode: charCode})
	require.Equal(t, http.StatusOK, putRes.StatusCode)
	_ = putRes.Body.Close()

	require.NoError(t, conn.Close())

	require.Eventually(t, func() bool {
		return getModelDisplay(t, client, ts.URL) == (api.ModelDisplayState{})
	}, time.Second, 10*time.Millisecond, "a disconnected display's stored character must actually be cleared, not just reported disconnected")
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

func TestServeModelDisplayWS_PushesCurrentStateOnConnect(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	putRes := setModelDisplay(t, client, ts.URL, api.SetModelDisplayCharCodeInput{CharCode: charCode, Flip: "x"})
	require.Equal(t, http.StatusOK, putRes.StatusCode)
	_ = putRes.Body.Close()

	// Connecting after the state already exists - a display that joins
	// mid-show must see what's already live immediately, not wait for
	// the next change to happen after it connects.
	conn := dialModelDisplayWS(t, client, ts.URL)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))

	var state api.ModelDisplayState
	require.NoError(t, conn.ReadJSON(&state))
	require.Equal(t, charCode, state.CharCode)
	require.Equal(t, "x", state.Flip)
}

func TestServeModelDisplayWS_PushesUpdatesLive(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	conn := dialModelDisplayWS(t, client, ts.URL)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))

	// Drains the initial state push (see PushesCurrentStateOnConnect
	// above) before triggering the real change this test cares about.
	var initial api.ModelDisplayState
	require.NoError(t, conn.ReadJSON(&initial))

	res := triggerModelDisplay(t, client, ts.URL)
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	var updated api.ModelDisplayState
	require.NoError(t, conn.ReadJSON(&updated), "a change must be pushed to an already-connected display immediately - there's no poll to wait for anymore")
	require.Equal(t, 1, updated.Trigger)
}

func TestServeModelDisplayWS_NonOwnerForbidden(t *testing.T) {
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

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/models/display/ws"
	_, res, err := (&websocket.Dialer{Jar: client.Jar}).Dial(wsURL, nil)
	require.Error(t, err, "the WebSocket handshake itself must fail for a non-owner, same as any other /api/models/... route")
	require.NotNil(t, res)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestServeModelDisplayWS_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithModelViewer(t, db)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/models/display/ws"
	_, res, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.Error(t, err)
	require.NotNil(t, res)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}
