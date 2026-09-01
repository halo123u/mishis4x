-- One row per sync-job check, not upserted in place - this is a history,
-- so price_cents gets its own row each time rather than overwriting
-- card_price_sources (which only tracks *where* to look, not what was
-- last found there). Surrogate id since multiple rows share the same
-- card_id, same pattern as game_matches.
--
-- price_cents is nullable - a checked page can come back with no listed
-- price at all (out of stock, delisted, or simply not found on the page
-- it's expected on), and that absence is itself meaningful data worth
-- recording, not an error to drop silently. Deliberately no separate
-- status column alongside it: *why* a price is missing is a source-
-- specific concept (TCG Republic's page distinguishes "Not Available"
-- from a card just not appearing at all; a different source might
-- represent availability completely differently, or not at all) that
-- this schema doesn't need to model - a null price already says
-- everything a caller needs ("nothing to report right now"), independent
-- of which source or why.
--
-- No FOREIGN KEY on card_id: history rows should outlive the card_id they
-- were recorded against even if the card is later deleted or reassigned
-- a fresh id (process-set --refresh) - this is priced-over-time data, not
-- something that should cascade away along with the catalog row.
CREATE TABLE card_price_history (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    card_id CHAR(36) NOT NULL,
    source VARCHAR(20) NOT NULL,
    price_cents INT NULL,
    recorded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_card_price_history_card_id (card_id)
);
