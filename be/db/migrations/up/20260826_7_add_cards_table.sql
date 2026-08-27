-- Same UUIDv7 ID rationale as sets (see 20260826_6_add_sets_table.sql).

CREATE TABLE cards (
    id CHAR(36) NOT NULL PRIMARY KEY,
    set_id CHAR(36) NOT NULL,
    name VARCHAR(256) NOT NULL,
    code VARCHAR(32) NOT NULL,
    rarity VARCHAR(32) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (set_id) REFERENCES sets(id),
    UNIQUE KEY uniq_cards_set_id_code (set_id, code),
    INDEX idx_cards_set_id (set_id)
);
