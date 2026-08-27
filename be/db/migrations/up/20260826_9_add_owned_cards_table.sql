-- quantity, not a boolean owned flag - duplicate counts matter for
-- lot-value decisions (this was a real, personally-hit problem tracking
-- duplicate card purchases by hand before this table existed).
--
-- Deliberately no set_id column here even though every card belongs to one -
-- card_id already implies set_id via cards.set_id, and storing it twice
-- risks the two disagreeing on some row later. Get a card's set through a
-- join on cards, not a denormalized copy here.

CREATE TABLE owned_cards (
    user_id INT NOT NULL,
    card_id CHAR(36) NOT NULL,
    quantity INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, card_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (card_id) REFERENCES cards(id)
);
