ALTER TABLE cards
    DROP FOREIGN KEY fk_cards_character_model_char_code,
    DROP INDEX idx_cards_character_model_char_code,
    DROP COLUMN character_model_char_code;
