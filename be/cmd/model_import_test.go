package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/mishis4x/persist"
	"github.com/stretchr/testify/require"
)

// testModelCharCode returns a short, unique-per-test char_code so
// parallel test runs never collide on the same primary key - same
// reasoning as persist's and handlers' own testCharCode helpers.
func testModelCharCode(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
}

// setModelAssetBaseURLForTest points modelAssetBaseURL at a fake server
// for the duration of one test, restoring the real value on cleanup -
// modelAssetBaseURL is a package var solely to make this possible (see
// its own doc comment).
func setModelAssetBaseURLForTest(t *testing.T, url string) {
	t.Helper()
	original := modelAssetBaseURL
	modelAssetBaseURL = url
	t.Cleanup(func() { modelAssetBaseURL = original })
}

// fakeModelAssetServer stands in for the real source viewer - real HTTP
// round trip against a fake server, not a mocked client, matching this
// codebase's testing philosophy elsewhere (see email/ebay's own fake
// servers). Serves char{charCode}.{skel,atlas,png} under /{charCode}/,
// mirroring the real viewer's own path shape closely enough for
// downloadModelAsset/importCharacterModel to exercise against.
func fakeModelAssetServer(t *testing.T, missingExt string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if missingExt != "" && len(r.URL.Path) > len(missingExt) && r.URL.Path[len(r.URL.Path)-len(missingExt):] == missingExt {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake asset bytes for " + r.URL.Path))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestDownloadModelAsset_Success(t *testing.T) {
	ts := fakeModelAssetServer(t, "")
	setModelAssetBaseURLForTest(t, ts.URL)
	client := ts.Client()

	data, err := downloadModelAsset(t.Context(), client, "002406", "skel")
	require.NoError(t, err)
	require.Contains(t, string(data), "002406/char002406.skel")
}

func TestDownloadModelAsset_NotFound(t *testing.T) {
	ts := fakeModelAssetServer(t, ".skel")
	setModelAssetBaseURLForTest(t, ts.URL)
	client := ts.Client()

	_, err := downloadModelAsset(t.Context(), client, "002406", "skel")
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}

func TestModelImport_StoresAllThreeAssets(t *testing.T) {
	db := testDB(t)
	p := &persist.Persist{DB: db}
	charCode := testModelCharCode(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})

	// modelImport always builds its own client pointed at the real
	// modelAssetBaseURL - importCharacterModel is exercised directly here
	// instead, against a fake server, the same way process_set_test.go
	// tests its own per-row helpers rather than the whole CLI Run func.
	ts := fakeModelAssetServer(t, "")
	client := ts.Client()
	setModelAssetBaseURLForTest(t, ts.URL)

	require.NoError(t, importCharacterModel(t.Context(), client, p, charCode))

	model, err := p.GetCharacterModel(t.Context(), charCode)
	require.NoError(t, err)
	require.Contains(t, string(model.Skeleton), ".skel")
	require.Contains(t, string(model.Atlas), ".atlas")
	require.Contains(t, string(model.Texture), ".png")
}

func TestModelImport_PartialFailureStoresNothing(t *testing.T) {
	db := testDB(t)
	p := &persist.Persist{DB: db}
	charCode := testModelCharCode(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})

	ts := fakeModelAssetServer(t, ".atlas")
	client := ts.Client()
	setModelAssetBaseURLForTest(t, ts.URL)

	require.Error(t, importCharacterModel(t.Context(), client, p, charCode))

	_, err := p.GetCharacterModel(t.Context(), charCode)
	require.ErrorIs(t, err, persist.ErrCharacterModelNotFound, "a character missing even one of its three files must not be stored at all")
}
