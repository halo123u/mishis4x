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
