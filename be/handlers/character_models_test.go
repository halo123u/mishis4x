package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/mishis4x/persist"
	"github.com/stretchr/testify/require"
)

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
