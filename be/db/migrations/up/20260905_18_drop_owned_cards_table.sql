-- Step 3 of 3 toward #108: owned_card_copies now fully replaces owned_cards
-- (every existing row was expanded into individual copies by the previous
-- migration) - drop the old table. No other table FKs to owned_cards, so
-- this is a clean drop, not a cascade.
DROP TABLE owned_cards;
