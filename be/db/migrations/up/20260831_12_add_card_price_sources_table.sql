-- Shared across every user, same as cards/card_images - not per-user
-- ownership data, so no user_id here. url is fully free-form/user-settable
-- (not reconstructed from a source-specific ID template) - `source` only
-- picks which parser the sync job applies to it (e.g. "tcg_republic"),
-- it's not itself part of the URL.
--
-- No API access exists yet for any of these sources (eBay's is pending
-- approval, TCG Republic doesn't have one at all), so this is deliberately
-- scrape-target metadata, not a cache of API credentials/endpoints.
--
-- last_checked_at is NULL until the sync job's first pass over a given
-- row, and is what the job itself uses to decide staleness (> 24h old) -
-- not any in-memory timer, so it stays correct across restarts/redeploys
-- of the process the sync job runs in.
--
-- ON DELETE CASCADE for the same reason as card_images: this is
-- re-enterable scrape config, not precious ownership data, so a card
-- losing its row on delete (or on process-set --refresh reassigning a
-- fresh id) just means re-adding the URL, not silent data loss.
CREATE TABLE card_price_sources (
    card_id CHAR(36) NOT NULL PRIMARY KEY,
    source VARCHAR(20) NOT NULL,
    url VARCHAR(500) NOT NULL,
    last_checked_at TIMESTAMP NULL,
    FOREIGN KEY (card_id) REFERENCES cards(id) ON DELETE CASCADE
);
