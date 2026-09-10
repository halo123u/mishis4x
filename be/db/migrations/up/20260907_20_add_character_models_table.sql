-- Stores the Spine (colloquially "Live2D" - they're actually different,
-- commonly-confused animation middleware products) model assets for a
-- curated allowlist of characters from one mobile game: skeleton/
-- animation data (.skel), the texture-region mapping (.atlas), and the
-- texture image itself. All three are unofficially-extracted, copyrighted
-- game assets (sourced via a fan viewer, not the game's publisher
-- directly) - this table is deliberately populated only by the
-- model-import CLI command against a hand-picked list of char_codes,
-- never by user-facing input, and every route that serves these back out
-- is gated to one account (see handlers.Data.ModelViewerUserID) the same
-- way collection-tracker's eBay-sourced data is - see that field's doc
-- comment for the parallel, independently-motivated (copyright, not eBay
-- ToS) reasoning.
--
-- char_code is the source viewer's own 6-digit internal character ID
-- (e.g. "002406") - the natural key here, not a surrogate id, same
-- convention as card_images keying on card_id. There's no FK to `cards`:
-- this ID space belongs to the source viewer, not this app's own catalog,
-- and nothing here requires a char_code to already correspond to a card
-- this app happens to track.
CREATE TABLE character_models (
    char_code VARCHAR(16) NOT NULL PRIMARY KEY,
    skeleton LONGBLOB NOT NULL,
    atlas LONGBLOB NOT NULL,
    texture LONGBLOB NOT NULL,
    texture_content_type VARCHAR(50) NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
