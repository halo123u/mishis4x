-- One reference image per catalog card, shared across every user - not
-- owned_cards (per-user data), this is closer to `cards` itself. Kept as
-- its own table rather than a column on `cards` so the LONGBLOB never
-- rides along on ordinary catalog queries (they already list explicit
-- columns, never SELECT *, but this makes that permanent by construction),
-- and so swapping to S3 later is just this one table becoming
-- (card_id, url) instead of (card_id, image) - `cards` itself never has to
-- change. No surrogate id: card_id is the natural key, same pattern as
-- owned_sets/owned_cards.
--
-- ON DELETE CASCADE is a deliberate difference from owned_cards, which
-- has no cascade (a card can't vanish out from under a user's real
-- ownership data - see TestDeleteCardsForSet_FailsIfACardIsOwned). An
-- image isn't precious the same way; it's re-fetchable from wherever it
-- came from, and process-set --refresh already deliberately reassigns
-- fresh card ids on a code-renumbering reimport, so cascading here just
-- means re-running with --images-dir afterward, not silent data loss.
CREATE TABLE card_images (
    card_id CHAR(36) NOT NULL PRIMARY KEY,
    image LONGBLOB NOT NULL,
    content_type VARCHAR(50) NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (card_id) REFERENCES cards(id) ON DELETE CASCADE
);
