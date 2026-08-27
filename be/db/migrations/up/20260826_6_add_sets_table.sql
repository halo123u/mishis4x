-- Uses a CHAR(36) UUIDv7 primary key instead of AUTO_INCREMENT, deliberately
-- - see issue #75. UUIDv7 embeds a millisecond timestamp prefix so IDs still
-- sort/insert like a sequential key (no InnoDB fragmentation), while
-- remaining globally unique and non-sequential.
--
-- Naming note: this file uses YYYYMMDD (not the legacy MMDDYYYY prefix on
-- earlier migrations) so it sorts lexically after them - MMDDYYYY breaks for
-- any month before November in a later year (e.g. "08262026" < "11092023"
-- as plain strings), which would have silently ordered this migration
-- before add_users_table/add_sessions_table on a fresh DB.

CREATE TABLE sets (
    id CHAR(36) NOT NULL PRIMARY KEY,
    name VARCHAR(256) NOT NULL,
    card_count INT NOT NULL DEFAULT 0,
    release_date DATE NULL,
    status VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
