-- Voice-line clips for a character model (see character_models' own
-- migration for the base asset set this supplements). Same source as
-- the skeleton/atlas/texture assets (jelosus2/BD2-L2D-Viewer), same
-- copyright/allowlist-only reasoning, same model-import CLI command -
-- this table is populated by the same command, not a separate one.
--
-- A separate table rather than more blob columns on character_models:
-- unlike skeleton/atlas/texture (exactly one of each per character),
-- audio is inherently multi-row per char_code - the source viewer
-- rotates through 3 numbered clips per language on each tap, and stores
-- two languages (JP/KR - confirmed against the real source, no English
-- track exists). (char_code, language, clip_index) is the natural
-- composite key: a real duplicate simply doesn't make sense here the way
-- a surrogate id would imply.
--
-- ON DELETE CASCADE (unlike cards.character_model_char_code's ON DELETE
-- SET NULL to its character_models row): audio only ever makes sense
-- alongside its character model, so if the model itself is ever removed,
-- there's no scenario where keeping orphaned voice clips around is
-- useful the way keeping a card's "this used to point somewhere" state
-- is.
CREATE TABLE character_model_audio (
    char_code VARCHAR(16) NOT NULL,
    language VARCHAR(2) NOT NULL,
    clip_index TINYINT UNSIGNED NOT NULL,
    audio LONGBLOB NOT NULL,
    content_type VARCHAR(50) NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (char_code, language, clip_index),
    CONSTRAINT fk_character_model_audio_char_code
        FOREIGN KEY (char_code) REFERENCES character_models(char_code) ON DELETE CASCADE
);
