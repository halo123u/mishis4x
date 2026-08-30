-- In cents, not a decimal dollar amount - avoids floating point rounding
-- issues on money entirely, at the cost of every reader needing to divide
-- by 100 for display. Nullable: not every owned card has a known purchase
-- price (a card onboarded before this column existed, or one that arrived
-- as part of an unpriced bulk lot) - NULL means "unknown", distinct from a
-- real $0 (e.g. a genuine giveaway/trade).
ALTER TABLE owned_cards ADD COLUMN price_paid_cents INT NULL AFTER quantity;
