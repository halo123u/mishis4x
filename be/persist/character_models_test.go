package persist

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testCharCode returns a short, unique-per-test char_code so parallel test
// runs never collide on the same primary key.
func testCharCode(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
}

func TestGetCharacterModel_NoneStoredReturnsNotFound(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	_, err := p.GetCharacterModel(t.Context(), testCharCode(t))
	require.ErrorIs(t, err, ErrCharacterModelNotFound, "an unimported char_code must not be an error, but must be distinguishable from a real one")
}

func TestUpsertCharacterModel_StoreAndUpdate(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	charCode := testCharCode(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})

	skeleton := []byte("not a real .skel, just test bytes")
	atlas := []byte("not a real .atlas, just test bytes")
	texture := []byte("not a real .png, just test bytes")
	require.NoError(t, p.UpsertCharacterModel(t.Context(), charCode, skeleton, atlas, texture, "image/png"))

	model, err := p.GetCharacterModel(t.Context(), charCode)
	require.NoError(t, err)
	require.Equal(t, skeleton, model.Skeleton)
	require.Equal(t, atlas, model.Atlas)
	require.Equal(t, texture, model.Texture)
	require.Equal(t, "image/png", model.TextureContentType)
	require.False(t, model.UpdatedAt.IsZero())

	// Re-running (a model-import re-run against an already-imported char
	// code) must replace in place, not error or leave the old bytes.
	updatedSkeleton := []byte("a different, updated skeleton")
	require.NoError(t, p.UpsertCharacterModel(t.Context(), charCode, updatedSkeleton, atlas, texture, "image/webp"))

	model, err = p.GetCharacterModel(t.Context(), charCode)
	require.NoError(t, err)
	require.Equal(t, updatedSkeleton, model.Skeleton)
	require.Equal(t, "image/webp", model.TextureContentType)
}

func TestCharacterModelExists_NotImported(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	exists, err := p.CharacterModelExists(t.Context(), testCharCode(t))
	require.NoError(t, err)
	require.False(t, exists)
}

func TestCharacterModelExists_Imported(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	charCode := testCharCode(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})
	require.NoError(t, p.UpsertCharacterModel(t.Context(), charCode, []byte("s"), []byte("a"), []byte("t"), "image/png"))

	exists, err := p.CharacterModelExists(t.Context(), charCode)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestListCharacterModels_ReturnsSortedCodes(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	base := testCharCode(t)
	codeA := base + "a"
	codeB := base + "b"
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code IN (?, ?)", codeA, codeB)
	})

	require.NoError(t, p.UpsertCharacterModel(t.Context(), codeB, []byte("s"), []byte("a"), []byte("t"), "image/png"))
	require.NoError(t, p.UpsertCharacterModel(t.Context(), codeA, []byte("s"), []byte("a"), []byte("t"), "image/png"))

	codes, err := p.ListCharacterModels(t.Context())
	require.NoError(t, err)
	require.Contains(t, codes, codeA)
	require.Contains(t, codes, codeB)

	// Both codes share the same random base, so their relative order in
	// the full list (which may include unrelated rows from other tests)
	// is the one thing this can assert without flaking - codeA < codeB.
	var indexA, indexB int
	for i, c := range codes {
		if c == codeA {
			indexA = i
		}
		if c == codeB {
			indexB = i
		}
	}
	require.Less(t, indexA, indexB, "results must be sorted by char_code")
}

func TestSetCardCharacterModel_LinkAndUnlink(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	charCode := testCharCode(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})
	require.NoError(t, p.UpsertCharacterModel(t.Context(), charCode, []byte("s"), []byte("a"), []byte("t"), "image/png"))

	setID, err := p.CreateSet(t.Context(), "SetCardCharacterModel Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})
	cardID, err := p.UpsertCard(t.Context(), setID, "Test Card", "TEST-001", "SR 1-star")
	require.NoError(t, err)

	require.NoError(t, p.SetCardCharacterModel(t.Context(), cardID, &charCode))

	cards, err := p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.NotNil(t, cards[0].CharacterModelCharCode)
	require.Equal(t, charCode, *cards[0].CharacterModelCharCode)

	// Re-setting to the exact same value it's already at (idempotent
	// retry, or the frontend re-selecting the same option) must not be
	// misreported as ErrCardNotFound - see SetCardCharacterModel's own
	// doc comment for why this is checked as a separate existence query
	// rather than trusting the UPDATE's own RowsAffected.
	require.NoError(t, p.SetCardCharacterModel(t.Context(), cardID, &charCode))

	require.NoError(t, p.SetCardCharacterModel(t.Context(), cardID, nil))
	cards, err = p.ListCardsBySet(t.Context(), setID)
	require.NoError(t, err)
	require.Nil(t, cards[0].CharacterModelCharCode, "unlinking must clear it, not just fail to error")

	// Unlinking an already-unlinked card is the same idempotent-retry
	// case as above, just at nil instead of a real value.
	require.NoError(t, p.SetCardCharacterModel(t.Context(), cardID, nil))
}

func TestSetCardCharacterModel_CardNotFound(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	err := p.SetCardCharacterModel(t.Context(), "does-not-exist", nil)
	require.ErrorIs(t, err, ErrCardNotFound)
}

func TestSetCardCharacterModel_UnknownCharCodeFailsOnFK(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	setID, err := p.CreateSet(t.Context(), "SetCardCharacterModel FK Test Set", 1, nil, "pending")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM cards WHERE set_id = ?", setID)
		_, _ = db.Exec("DELETE FROM sets WHERE id = ?", setID)
	})
	cardID, err := p.UpsertCard(t.Context(), setID, "Test Card", "TEST-002", "SR 1-star")
	require.NoError(t, err)

	neverImported := testCharCode(t)
	err = p.SetCardCharacterModel(t.Context(), cardID, &neverImported)
	require.Error(t, err, "a char_code with no character_models row must fail via the FK, not silently link")
}
