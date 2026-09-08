-- Links a catalog card to the character model that should render for it
-- (be/persist/character_models.go) - nullable, since most cards won't have
-- a model imported at all (model-import's own allowlist is deliberately
-- small, see its doc comment), and a real FK rather than a loose string:
-- a card can only point at a char_code that's actually been imported.
-- Several cards commonly point at the same char_code (the same BD2
-- character drawn at different rarities is usually the same model), so
-- this is many-to-one, not a join table - a single card never needs more
-- than one model at a time.
--
-- ON DELETE SET NULL, not CASCADE and not a hard block: unlike
-- owned_cards' relationship to cards (real ownership data that must never
-- be silently orphaned), losing this link if a character_models row gets
-- deleted just means the card stops showing a "view model" link - an
-- easy, non-destructive thing to reattach later, not data worth blocking
-- a delete over.
ALTER TABLE cards
    ADD COLUMN character_model_char_code VARCHAR(16) NULL,
    ADD CONSTRAINT fk_cards_character_model_char_code
        FOREIGN KEY (character_model_char_code) REFERENCES character_models(char_code) ON DELETE SET NULL,
    ADD INDEX idx_cards_character_model_char_code (character_model_char_code);
