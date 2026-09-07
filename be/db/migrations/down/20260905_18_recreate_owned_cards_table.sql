-- Best-effort reversal, not a data restore - matches every other down
-- file's convention (e.g. 20260830_10's down just drops a column, losing
-- whatever was in it) rather than trying to recompute quantity/price back
-- out of owned_card_copies. Recreates an empty shell of the pre-#108 shape
-- so a full down run leaves a structurally valid, if empty, owned_cards.
CREATE TABLE IF NOT EXISTS owned_cards (
    user_id INT NOT NULL,
    card_id CHAR(36) NOT NULL,
    quantity INT NOT NULL DEFAULT 0,
    price_paid_cents INT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, card_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (card_id) REFERENCES cards(id)
);
