package persist

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCharacterModelAudioClip_NoneStoredReturnsNotFound(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	_, err := p.GetCharacterModelAudioClip(t.Context(), testCharCode(t), "JP", 1)
	require.ErrorIs(t, err, ErrCharacterModelAudioNotFound, "an unimported clip must not be an error, but must be distinguishable from a real one")
}

func TestUpsertCharacterModelAudio_StoreAndUpdate(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	charCode := testCharCode(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_model_audio WHERE char_code = ?", charCode)
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})
	require.NoError(t, p.UpsertCharacterModel(t.Context(), charCode, []byte("s"), []byte("a"), []byte("t"), "image/png"))

	audio := []byte("not a real .webm, just test bytes")
	require.NoError(t, p.UpsertCharacterModelAudio(t.Context(), charCode, "JP", 1, audio, "audio/webm"))

	clip, err := p.GetCharacterModelAudioClip(t.Context(), charCode, "JP", 1)
	require.NoError(t, err)
	require.Equal(t, audio, clip.Audio)
	require.Equal(t, "audio/webm", clip.ContentType)
	require.False(t, clip.UpdatedAt.IsZero())

	// Re-running (a model-import re-run) must replace in place, not error
	// or leave the old bytes - same re-run safety as UpsertCharacterModel.
	updated := []byte("a different, updated clip")
	require.NoError(t, p.UpsertCharacterModelAudio(t.Context(), charCode, "JP", 1, updated, "audio/webm"))

	clip, err = p.GetCharacterModelAudioClip(t.Context(), charCode, "JP", 1)
	require.NoError(t, err)
	require.Equal(t, updated, clip.Audio)
}

func TestUpsertCharacterModelAudio_DistinctLanguageAndIndexAreSeparateRows(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}
	charCode := testCharCode(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM character_model_audio WHERE char_code = ?", charCode)
		_, _ = db.Exec("DELETE FROM character_models WHERE char_code = ?", charCode)
	})
	require.NoError(t, p.UpsertCharacterModel(t.Context(), charCode, []byte("s"), []byte("a"), []byte("t"), "image/png"))

	require.NoError(t, p.UpsertCharacterModelAudio(t.Context(), charCode, "JP", 1, []byte("jp-1"), "audio/webm"))
	require.NoError(t, p.UpsertCharacterModelAudio(t.Context(), charCode, "JP", 2, []byte("jp-2"), "audio/webm"))
	require.NoError(t, p.UpsertCharacterModelAudio(t.Context(), charCode, "KR", 1, []byte("kr-1"), "audio/webm"))

	jp1, err := p.GetCharacterModelAudioClip(t.Context(), charCode, "JP", 1)
	require.NoError(t, err)
	require.Equal(t, []byte("jp-1"), jp1.Audio)

	jp2, err := p.GetCharacterModelAudioClip(t.Context(), charCode, "JP", 2)
	require.NoError(t, err)
	require.Equal(t, []byte("jp-2"), jp2.Audio)

	kr1, err := p.GetCharacterModelAudioClip(t.Context(), charCode, "KR", 1)
	require.NoError(t, err)
	require.Equal(t, []byte("kr-1"), kr1.Audio)
}

func TestUpsertCharacterModelAudio_UnknownCharCodeFailsOnFK(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	err := p.UpsertCharacterModelAudio(t.Context(), testCharCode(t), "JP", 1, []byte("a"), "audio/webm")
	require.Error(t, err, "audio for a char_code with no character_models row must fail via the FK, not silently store")
}
