-- Per-user "I've onboarded this set" gate, separate from card-level
-- ownership in owned_cards. user_id stays INT to match users.id (see #76
-- for the separate, deliberately-deferred task of moving that to UUIDv7
-- too) - set_id is the new table's own CHAR(36) UUIDv7 key.
--
-- No surrogate id column: this is a pure ownership join, the natural key
-- (user_id, set_id) is the primary key.

CREATE TABLE owned_sets (
    user_id INT NOT NULL,
    set_id CHAR(36) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, set_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (set_id) REFERENCES sets(id)
);
