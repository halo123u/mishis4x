package cmd

import (
	"database/sql"
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

// createTestSetAndCardForLinking creates a fresh set (with a unique-ish
// name) and one card in it, returning the set's name (what
// linkCardsToCharacterModel resolves by) and the card's code.
func createTestSetAndCardForLinking(t *testing.T, db *sql.DB) (setName, cardCode string) {
	t.Helper()
	p := &persist.Persist{DB: db}

	setName = "model-import link test set " + testModelCharCode(t)
	setID, err := p.CreateSet(t.Context(), setName, 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})

	cardCode = "TEST-" + testModelCharCode(t)
	_, err = p.CreateCard(t.Context(), setID, "Test Card", cardCode, "SR 1-star")
	require.NoError(t, err)

	return setName, cardCode
}

func TestLinkCardsToCharacterModel_LinksMatchingCards(t *testing.T) {
	db := testDB(t)
	p := &persist.Persist{DB: db}
	charCode := testModelCharCode(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})
	require.NoError(t, p.UpsertCharacterModel(t.Context(), charCode, []byte("s"), []byte("a"), []byte("t"), "image/png"))

	setName, cardCode := createTestSetAndCardForLinking(t, db)

	linkCardsToCharacterModel(t.Context(), p, setName, charCode, []string{cardCode})

	setID, err := p.GetSetIDByName(t.Context(), setName)
	require.NoError(t, err)
	cards, err := p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.NotNil(t, cards[0].CharacterModelCharCode)
	require.Equal(t, charCode, *cards[0].CharacterModelCharCode)
}

func TestLinkCardsToCharacterModel_UnknownCardCodeSkipsNotFatal(t *testing.T) {
	db := testDB(t)
	p := &persist.Persist{DB: db}
	charCode := testModelCharCode(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})
	require.NoError(t, p.UpsertCharacterModel(t.Context(), charCode, []byte("s"), []byte("a"), []byte("t"), "image/png"))

	setName, realCode := createTestSetAndCardForLinking(t, db)

	// One code that doesn't exist, alongside one that does - the bad one
	// must not stop the good one from linking (same per-item tolerance as
	// model-import's own asset downloads).
	linkCardsToCharacterModel(t.Context(), p, setName, charCode, []string{"DOES-NOT-EXIST", realCode})

	setID, err := p.GetSetIDByName(t.Context(), setName)
	require.NoError(t, err)
	cards, err := p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.NotNil(t, cards[0].CharacterModelCharCode, "the real card code must still link despite the bad one earlier in the list")
}

func TestLinkCardsToCharacterModel_UnknownSetDoesNotPanic(t *testing.T) {
	db := testDB(t)
	p := &persist.Persist{DB: db}
	charCode := testModelCharCode(t)

	// Just needs to not panic/crash - there's nothing else to assert
	// against when the set itself doesn't resolve.
	linkCardsToCharacterModel(t.Context(), p, "no such set at all", charCode, []string{"whatever"})
}

func TestLinkCardIDsToCharacterModel_LinksMatchingCards(t *testing.T) {
	db := testDB(t)
	p := &persist.Persist{DB: db}
	charCode := testModelCharCode(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})
	require.NoError(t, p.UpsertCharacterModel(t.Context(), charCode, []byte("s"), []byte("a"), []byte("t"), "image/png"))

	setName, cardCode := createTestSetAndCardForLinking(t, db)
	setID, err := p.GetSetIDByName(t.Context(), setName)
	require.NoError(t, err)
	cardID, err := p.GetCardIDByCode(t.Context(), setID, cardCode)
	require.NoError(t, err)

	linkCardIDsToCharacterModel(t.Context(), p, charCode, []string{cardID})

	cards, err := p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.NotNil(t, cards[0].CharacterModelCharCode)
	require.Equal(t, charCode, *cards[0].CharacterModelCharCode)
}

func TestLinkCardIDsToCharacterModel_UnknownIDSkipsNotFatal(t *testing.T) {
	db := testDB(t)
	p := &persist.Persist{DB: db}
	charCode := testModelCharCode(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})
	require.NoError(t, p.UpsertCharacterModel(t.Context(), charCode, []byte("s"), []byte("a"), []byte("t"), "image/png"))

	setName, cardCode := createTestSetAndCardForLinking(t, db)
	setID, err := p.GetSetIDByName(t.Context(), setName)
	require.NoError(t, err)
	realID, err := p.GetCardIDByCode(t.Context(), setID, cardCode)
	require.NoError(t, err)

	// A bad id (well-formed enough to not error building the query, just
	// not matching any real card) must not stop the real one from
	// linking - same tolerance as linkCardsToCharacterModel's own
	// unknown-code case.
	linkCardIDsToCharacterModel(t.Context(), p, charCode, []string{"does-not-exist", realID})

	cards, err := p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.NotNil(t, cards[0].CharacterModelCharCode, "the real card id must still link despite the bad one earlier in the list")
}
