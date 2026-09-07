-- Step 1 of 3 (expand/backfill/contract, same discipline as #76) toward
-- issue #108: owned_cards currently stores one row per (user_id, card_id)
-- with an integer quantity and a single price_paid_cents covering the
-- whole quantity - so "3 copies, bought separately for $10/$15/$12"
-- collapses into one ambiguous number. This table switches to one row
-- PER PHYSICAL COPY instead: quantity becomes a COUNT(*) at read time
-- (see persist.GetOwnedCard/ListOwnedCardsBySet), and each copy carries
-- its own price_paid_cents.
--
-- id is a surrogate key (UUIDv7, persist.NewUUIDv7 - same convention as
-- cards/sets, see #75) rather than (user_id, card_id), since that pair is
-- no longer unique - a user can now have many rows for the same card. New
-- rows' UUIDv7 ids are time-ordered, but the one-time backfill migration
-- that follows this one populates its rows via plain SQL, using MySQL's
-- UUID() (a v1 UUID, not v7 - its hex form doesn't sort chronologically
-- alongside a real UUIDv7's), so id ordering alone can't be trusted to
-- mean insertion order across backfilled and live rows. created_at is what
-- actually anchors "most recently added copy" (see
-- reconcileOwnedCardCopies) - id only breaks ties within it.
--
-- No unique constraint on (user_id, card_id) - by design, multiplicity is
-- the whole point here.
CREATE TABLE owned_card_copies (
    id CHAR(36) NOT NULL,
    user_id INT NOT NULL,
    card_id CHAR(36) NOT NULL,
    price_paid_cents INT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    INDEX idx_owned_card_copies_user_card (user_id, card_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (card_id) REFERENCES cards(id)
);
