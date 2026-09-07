-- Step 2 of 3 toward #108: expands every existing owned_cards row into
-- `quantity` individual owned_card_copies rows. All copies of a given
-- existing row inherit that row's price_paid_cents split the same way a
-- fresh persist.SetOwnedCards call would split it going forward (see
-- reconcileOwnedCardCopies): the whole recorded amount lands on exactly
-- one copy (n = 1) and the rest backfill with an unknown (NULL) price -
-- there's no real per-copy purchase history to recover it from, but this
-- keeps SUM(price_paid_cents) across a card's copies exactly equal to what
-- was already recorded, with no double-counting and no data loss.
--
-- The recursive `seq` CTE stands in for a numbers table MySQL doesn't have
-- built in, capped at 500 - nobody realistically owns 500+ copies of one
-- card, and it stays comfortably under the 8.0 default
-- cte_max_recursion_depth of 1000.
INSERT INTO owned_card_copies (id, user_id, card_id, price_paid_cents)
WITH RECURSIVE seq (n) AS (
    SELECT 1
    UNION ALL
    SELECT n + 1 FROM seq WHERE n < 500
)
SELECT UUID(), oc.user_id, oc.card_id, IF(seq.n = 1, oc.price_paid_cents, NULL)
FROM owned_cards oc
JOIN seq ON seq.n <= oc.quantity
WHERE oc.quantity > 0;
