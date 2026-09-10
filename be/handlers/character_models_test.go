package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/mishis4x/api"
	"example.com/mishis4x/persist"
	"github.com/stretchr/testify/require"
)

// putJSON mirrors postJSON (users_test.go) but for a PUT request, and
// takes any JSON-marshalable body rather than just map[string]string -
// api.SetCardCharacterModelInput's CharCode is a *string, which
// postJSON's map[string]string can't represent.
func putJSON(t *testing.T, client *http.Client, url string, body any) *http.Response {
	t.Helper()

	b, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })

	return res
}

// testCharCode returns a short, unique-per-test char_code so parallel test
// runs never collide on the same primary key - mirrors persist package's
// own helper of the same name (this package can't import a _test.go
// symbol from another package).
func testCharCode(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
}

// newTestServerWithModelViewer builds a server with modelViewerUserID
// recognized as the one account allowed to see /api/models/... routes
// (see ModelViewerUserID's doc comment), then creates and logs in as
// exactly that user - the returned client is already authenticated and
// ready to hit those routes without a separate login step in every test.
func newTestServerWithModelViewer(t *testing.T, db *sql.DB) (*httptest.Server, *http.Client) {
	t.Helper()

	username := testUsername(t, db)
	modelViewerUserID := createTestUser(t, db, username, "correctpass123")

	d := newTestDataWithModelViewer(db, modelViewerUserID)
	ts := httptest.NewServer(d.NewRouter())
	t.Cleanup(ts.Close)

	client := newClient(t)
	res := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	return ts, client
}

func storeTestCharacterModel(t *testing.T, db *sql.DB, charCode string) {
	t.Helper()
	p := &persist.Persist{DB: db}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})
	require.NoError(t, p.UpsertCharacterModel(t.Context(), charCode,
		[]byte("not a real .skel, just test bytes"),
		[]byte("not a real .atlas, just test bytes"),
		[]byte("\x89PNG\r\n\x1a\n not a real png, just test bytes"),
		"image/png",
	))
}

func TestListCharacterModels_ReturnsStoredCodes(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	res, err := client.Get(ts.URL + "/api/models")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var codes []string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&codes))
	require.Contains(t, codes, charCode)
}

func TestListCharacterModels_NonOwnerForbidden(t *testing.T) {
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

	res, err := client.Get(ts.URL + "/api/models")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestListCharacterModels_Unauthenticated(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithModelViewer(t, db)

	res, err := http.Get(ts.URL + "/api/models")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestGetCharacterModelAssets_StreamsStoredBytes(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	skelRes, err := client.Get(ts.URL + "/api/models/" + charCode + "/skeleton")
	require.NoError(t, err)
	defer func() { _ = skelRes.Body.Close() }()
	require.Equal(t, http.StatusOK, skelRes.StatusCode)
	require.Equal(t, "application/octet-stream", skelRes.Header.Get("Content-Type"))

	atlasRes, err := client.Get(ts.URL + "/api/models/" + charCode + "/atlas")
	require.NoError(t, err)
	defer func() { _ = atlasRes.Body.Close() }()
	require.Equal(t, http.StatusOK, atlasRes.StatusCode)

	textureRes, err := client.Get(ts.URL + "/api/models/" + charCode + "/texture")
	require.NoError(t, err)
	defer func() { _ = textureRes.Body.Close() }()
	require.Equal(t, http.StatusOK, textureRes.StatusCode)
	require.Equal(t, "image/png", textureRes.Header.Get("Content-Type"))
}

func TestGetCharacterModelAssets_NotImportedReturnsNotFound(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	res, err := client.Get(ts.URL + "/api/models/999999/skeleton")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestGetCharacterModelAssets_NonOwnerForbidden(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	username := testUsername(t, db)
	createTestUser(t, db, username, "correctpass123")
	client := newClient(t)
	loginRes := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, loginRes.StatusCode)

	res, err := client.Get(ts.URL + "/api/models/" + charCode + "/skeleton")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestGetCharacterModelAudio_StreamsStoredClip(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)
	p := &persist.Persist{DB: db}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_model_audio WHERE char_code = ?", charCode)
	})
	require.NoError(t, p.UpsertCharacterModelAudio(t.Context(), charCode, "JP", 1, []byte("not a real .webm, just test bytes"), "audio/webm"))

	res, err := client.Get(ts.URL + "/api/models/" + charCode + "/audio/JP/1")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "audio/webm", res.Header.Get("Content-Type"))
}

func TestGetCharacterModelAudio_NotImportedReturnsNotFound(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	// The model itself exists, but no clip was ever stored for this
	// language/index - the ordinary "this character has fewer clips than
	// the frontend probed for" case, not an error.
	res, err := client.Get(ts.URL + "/api/models/" + charCode + "/audio/JP/3")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestGetCharacterModelAudio_InvalidClipIndexBadRequest(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)

	res, err := client.Get(ts.URL + "/api/models/" + charCode + "/audio/JP/not-a-number")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestGetCharacterModelAudio_NonOwnerForbidden(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)
	p := &persist.Persist{DB: db}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_model_audio WHERE char_code = ?", charCode)
	})
	require.NoError(t, p.UpsertCharacterModelAudio(t.Context(), charCode, "JP", 1, []byte("a"), "audio/webm"))

	username := testUsername(t, db)
	createTestUser(t, db, username, "correctpass123")
	client := newClient(t)
	loginRes := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, loginRes.StatusCode)

	res, err := client.Get(ts.URL + "/api/models/" + charCode + "/audio/JP/1")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

// createTestCardForModelLink creates a fresh set+card, returning both
// ids - the setup every SetCardCharacterModel test needs, factored out
// since none of them care about the set/card themselves beyond having a
// real cardID to link against (setID is only needed to read the card
// back via GET /api/sets/{setID}/cards to confirm a link took effect).
func createTestCardForModelLink(t *testing.T, db *sql.DB) (setID, cardID string) {
	t.Helper()
	p := &persist.Persist{DB: db}

	setID, err := p.CreateSet(t.Context(), "SetCardCharacterModel Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardID, err = p.CreateCard(t.Context(), setID, "Test Card", "TEST-"+testCharCode(t), "SR 1-star")
	require.NoError(t, err)

	return setID, cardID
}

func TestSetCardCharacterModel_LinksCard(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)
	setID, cardID := createTestCardForModelLink(t, db)

	res := putJSON(t, client, ts.URL+"/api/models/cards/"+cardID, api.SetCardCharacterModelInput{CharCode: &charCode})
	require.Equal(t, http.StatusOK, res.StatusCode)

	getRes, err := client.Get(ts.URL + "/api/sets/" + setID + "/cards")
	require.NoError(t, err)
	defer func() { _ = getRes.Body.Close() }()
	var cards []api.Card
	require.NoError(t, json.NewDecoder(getRes.Body).Decode(&cards))
	require.Len(t, cards, 1)
	require.NotNil(t, cards[0].CharacterModelCharCode)
	require.Equal(t, charCode, *cards[0].CharacterModelCharCode)
}

func TestSetCardCharacterModel_Unlink(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)
	charCode := testCharCode(t)
	storeTestCharacterModel(t, db, charCode)
	setID, cardID := createTestCardForModelLink(t, db)

	res := putJSON(t, client, ts.URL+"/api/models/cards/"+cardID, api.SetCardCharacterModelInput{CharCode: &charCode})
	require.Equal(t, http.StatusOK, res.StatusCode)

	unlinkRes := putJSON(t, client, ts.URL+"/api/models/cards/"+cardID, api.SetCardCharacterModelInput{CharCode: nil})
	require.Equal(t, http.StatusOK, unlinkRes.StatusCode)

	getRes, err := client.Get(ts.URL + "/api/sets/" + setID + "/cards")
	require.NoError(t, err)
	defer func() { _ = getRes.Body.Close() }()
	var cards []api.Card
	require.NoError(t, json.NewDecoder(getRes.Body).Decode(&cards))
	require.Len(t, cards, 1)
	require.Nil(t, cards[0].CharacterModelCharCode, "unlinking must clear it, not just fail to error")
}

func TestSetCardCharacterModel_CardNotFound(t *testing.T) {
	db := testDB(t)
	ts, client := newTestServerWithModelViewer(t, db)

	res := putJSON(t, client, ts.URL+"/api/models/cards/does-not-exist", api.SetCardCharacterModelInput{CharCode: nil})
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestSetCardCharacterModel_NonOwnerForbidden(t *testing.T) {
	db := testDB(t)
	ts, _ := newTestServerWithModelViewer(t, db)
	_, cardID := createTestCardForModelLink(t, db)

	username := testUsername(t, db)
	createTestUser(t, db, username, "correctpass123")
	client := newClient(t)
	loginRes := postJSON(t, client, ts.URL+"/api/user/login", map[string]string{
		"username": username,
		"password": "correctpass123",
	})
	require.Equal(t, http.StatusOK, loginRes.StatusCode)

	res := putJSON(t, client, ts.URL+"/api/models/cards/"+cardID, api.SetCardCharacterModelInput{CharCode: nil})
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}
