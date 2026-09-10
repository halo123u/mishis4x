package persist

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
)

// ErrCharacterModelAudioNotFound is returned by GetCharacterModelAudioClip
// when no clip is stored for the requested (charCode, language,
// clipIndex) - the ordinary state for the vast majority of that space
// (most characters only have 3 clips per language, not an arbitrary
// number), not a data-integrity problem.
var ErrCharacterModelAudioNotFound = errors.New("character model audio not found")

// CharacterModelAudioClip is one voice-line clip, as returned by
// GetCharacterModelAudioClip. ContentType is stored alongside the audio
// bytes for the same reason CharacterModel.TextureContentType is: the
// source viewer serves plain .webm files with no other way to recover
// the MIME type at request time.
type CharacterModelAudioClip struct {
	Audio       []byte
	ContentType string
	UpdatedAt   time.Time
}

// UpsertCharacterModelAudio stores (or replaces, on a re-run of
// model-import) one voice-line clip for charCode - safe to call
// repeatedly, same re-run safety as UpsertCharacterModel. charCode must
// already have a character_models row (enforced by this table's own FK,
// not checked separately here - same "let the database enforce it"
// convention as SetCardCharacterModel's char_code FK).
func (p *Persist) UpsertCharacterModelAudio(ctx context.Context, charCode, language string, clipIndex int, audio []byte, contentType string) error {
	_, err := sq.Insert("character_model_audio").
		Columns("char_code", "language", "clip_index", "audio", "content_type").
		Values(charCode, language, clipIndex, audio, contentType).
		Suffix("ON DUPLICATE KEY UPDATE audio = VALUES(audio), content_type = VALUES(content_type)").
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}

// GetCharacterModelAudioClip returns one stored voice-line clip. Returns
// ErrCharacterModelAudioNotFound if no clip is stored at that
// (charCode, language, clipIndex) - the frontend is expected to probe
// clip_index 1, 2, 3 and stop at the first 404 (mirroring the source
// viewer's own hardcoded 3-clip rotation), so this is an ordinary,
// expected outcome for clip_index 4+ or an unimported char_code, not
// something worth a heavier "list what's available" round trip first.
func (p *Persist) GetCharacterModelAudioClip(ctx context.Context, charCode, language string, clipIndex int) (CharacterModelAudioClip, error) {
	row := sq.Select("audio", "content_type", "updated_at").
		From("character_model_audio").
		Where(sq.Eq{"char_code": charCode, "language": language, "clip_index": clipIndex}).
		RunWith(p.DB).
		QueryRowContext(ctx)

	var c CharacterModelAudioClip
	if err := row.Scan(&c.Audio, &c.ContentType, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CharacterModelAudioClip{}, ErrCharacterModelAudioNotFound
		}
		return CharacterModelAudioClip{}, err
	}

	return c, nil
}
