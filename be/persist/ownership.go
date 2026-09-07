package persist

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rs/zerolog/log"
)

type OwnedSet struct {
	UserID    int
	SetID     string
	CreatedAt time.Time
}

// OwnedCard is the read-side aggregate view of a user's ownership of one
// card - Quantity and PricePaidCents are both computed from
// owned_card_copies (COUNT(*) and SUM(price_paid_cents) respectively, see
// #108), not stored columns of their own. UpdatedAt is the newest copy's
// created_at, standing in for the old single-row updated_at.
type OwnedCard struct {
	UserID         int
	CardID         string
	Quantity       int
	PricePaidCents *int
	UpdatedAt      time.Time
}

// SetOwnedSet marks setID as onboarded for userID. Idempotent - calling it
// again for a set the user has already onboarded is a no-op, not an error.
func (p *Persist) SetOwnedSet(ctx context.Context, userID int, setID string) error {
	_, err := sq.Insert("owned_sets").
		Columns("user_id", "set_id").
		Values(userID, setID).
		Suffix("ON DUPLICATE KEY UPDATE set_id = set_id").
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}

// ListOwnedSets returns the full Set data for every set userID has
// onboarded, ordered by name. Distinct from ListSets (every set in the
// catalog) - this is what the collection dashboard actually shows, since a
// fresh user's onboarded list starts empty even when the catalog doesn't.
func (p *Persist) ListOwnedSets(ctx context.Context, userID int) ([]Set, error) {
	rows, err := sq.Select("s.id", "s.name", "s.card_count", "s.release_date", "s.status", "s.created_at").
		From("owned_sets os").
		Join("sets s ON s.id = os.set_id").
		Where(sq.Eq{"os.user_id": userID}).
		OrderBy("s.name").
		RunWith(p.DB).
		QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing rows")
		}
	}()

	var sets []Set
	for rows.Next() {
		var s Set
		var releaseDate sql.NullTime
		if err := rows.Scan(&s.ID, &s.Name, &s.CardCount, &releaseDate, &s.Status, &s.CreatedAt); err != nil {
			return nil, err
		}
		if releaseDate.Valid {
			s.ReleaseDate = &releaseDate.Time
		}
		sets = append(sets, s)
	}

	return sets, rows.Err()
}

// ListOwnedSetIDs returns the IDs of every set userID has onboarded.
func (p *Persist) ListOwnedSetIDs(ctx context.Context, userID int) ([]string, error) {
	rows, err := sq.Select("set_id").
		From("owned_sets").
		Where(sq.Eq{"user_id": userID}).
		RunWith(p.DB).
		QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing rows")
		}
	}()

	var setIDs []string
	for rows.Next() {
		var setID string
		if err := rows.Scan(&setID); err != nil {
			return nil, err
		}
		setIDs = append(setIDs, setID)
	}

	return setIDs, rows.Err()
}

// DeleteOwnedSet removes setID from userID's collection entirely - the
// owned_sets row (so it drops off the dashboard and becomes onboardable
// again) and every owned_card_copies row for one of setID's cards (so
// re-adding the set later starts from a clean slate instead of
// resurrecting old ownership data via the editor form's pre-fill). A
// no-op, not an error, if userID never onboarded setID - same idempotent
// shape as SetOwnedSet.
func (p *Persist) DeleteOwnedSet(ctx context.Context, userID int, setID string) error {
	_, err := sq.Delete("owned_card_copies").
		Where("card_id IN (SELECT id FROM cards WHERE set_id = ?)", setID).
		Where(sq.Eq{"user_id": userID}).
		RunWith(p.DB).
		ExecContext(ctx)
	if err != nil {
		return err
	}

	_, err = sq.Delete("owned_sets").
		Where(sq.Eq{"user_id": userID, "set_id": setID}).
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}

// SetCardQuantity reconciles how many owned_card_copies rows exist for
// (userID, cardID) to quantity - inserting new (always nil-priced) copies
// if it went up, deleting the most-recently-added ones first if it went
// down (see reconcileOwnedCardCopies). Has no price of its own to set, so
// setPrice=false: existing copies' prices are never touched, matching this
// function's pre-#108 contract (it only ever upserted the quantity
// column). A quantity of 0 is a valid state (all copies removed), distinct
// from a card that was never interacted with (no rows either way, but see
// ListOwnedCardsBySet's doc comment for what that means for querying it).
func (p *Persist) SetCardQuantity(ctx context.Context, userID int, cardID string, quantity int) error {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			log.Error().Err(rollbackErr).Msg("error rolling back set card quantity transaction")
		}
	}()

	if err := reconcileOwnedCardCopies(ctx, tx, userID, cardID, quantity, nil, false); err != nil {
		return err
	}

	return tx.Commit()
}

// CardCopy is one physical copy of a card - the unit CardQuantity.Copies
// operates on. ID is empty for a copy that doesn't exist yet (a request to
// create one); non-empty to reference an existing owned_card_copies row.
// See reconcileOwnedCardCopiesByID for exactly how a []CardCopy gets
// reconciled against what's actually stored.
type CardCopy struct {
	ID             string
	PricePaidCents *int
}

// CardQuantity pairs a card with either a plain target quantity or a full
// per-copy list - the unit SetOwnedCards operates on in bulk, as opposed to
// SetCardQuantity's one-card-at-a-time form.
//
// Quantity/PricePaidCents are the legacy aggregate write form - still what
// SetCardQuantity uses, and still what's returned on read (a plain
// len()/sum() over Copies - see ListOwnedCardsBySet/GetOwnedCard).
//
// Copies, when non-nil (#108), is authoritative on write: the caller's
// full desired end state for this card's physical copies, in whatever
// order they should be numbered (oldest first is the convention every
// current caller uses - see ListOwnedCardsBySet). Reconciled by identity
// (reconcileOwnedCardCopiesByID), not just by count - an existing copy
// whose ID isn't present gets deleted, one that is present gets its price
// updated to match, and an empty-ID entry gets created fresh. This is what
// makes editing an arbitrary older copy's price (not just "whichever is
// newest") possible, and it sidesteps reconcileOwnedCardCopies' LIFO
// guess-which-copy-to-delete problem entirely: the caller already knows
// exactly which copies survive, because it's the one showing them to the
// user. When Copies is nil, Quantity/PricePaidCents drive the legacy
// count-only reconciliation instead (reconcileOwnedCardCopies) - nil, not
// just empty, is the signal: an empty-but-non-nil Copies is a real "delete
// every copy of this card" request.
type CardQuantity struct {
	CardID         string
	Quantity       int
	PricePaidCents *int
	Copies         []CardCopy
}

// SetOwnedCards reconciles every entry in cards - the bulk form of
// SetCardQuantity, used by the onboarding flow's card-selection step,
// where a user submits many cards' ownership at once rather than one at a
// time. Each entry with a non-nil Copies is reconciled by identity
// (reconcileOwnedCardCopiesByID); everything else falls back to the
// legacy count-only form (reconcileOwnedCardCopies). All cards are
// reconciled in one transaction (a failure partway through rolls back
// every card in this call, not just the one that failed). A nil/empty
// cards is a no-op, not an error - it never opens a transaction for
// nothing.
func (p *Persist) SetOwnedCards(ctx context.Context, userID int, cards []CardQuantity) error {
	if len(cards) == 0 {
		return nil
	}

	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			log.Error().Err(rollbackErr).Msg("error rolling back set owned cards transaction")
		}
	}()

	for _, c := range cards {
		if c.Copies != nil {
			if err := reconcileOwnedCardCopiesByID(ctx, tx, userID, c.CardID, c.Copies); err != nil {
				return err
			}
			continue
		}
		if err := reconcileOwnedCardCopies(ctx, tx, userID, c.CardID, c.Quantity, c.PricePaidCents, true); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// reconcileOwnedCardCopiesByID reconciles (userID, cardID)'s
// owned_card_copies rows to exactly match copies, by identity rather than
// just count (#108's real per-copy editing capability - see CardQuantity's
// doc comment): any existing row whose ID isn't present in copies is
// deleted, each entry with an ID matching an existing row updates that
// row's price, and each entry with no ID (empty string) creates a new
// copy. copies == nil is not valid here - an empty-but-non-nil slice is a
// real "own zero copies of this card" request; callers that mean "don't
// touch copies at all" should use reconcileOwnedCardCopies instead.
//
// An ID in copies that doesn't match any of userID's existing rows for
// cardID (forged, stale, or belonging to a different user/card entirely)
// is silently ignored rather than erroring - the scoped
// user_id/card_id/id WHERE clause below means it simply can't affect a
// row it doesn't own, so there's nothing unsafe about treating it as a
// no-op rather than failing the whole request over it.
func reconcileOwnedCardCopiesByID(ctx context.Context, runner sq.BaseRunner, userID int, cardID string, copies []CardCopy) error {
	rows, err := sq.Select("id").
		From("owned_card_copies").
		Where(sq.Eq{"user_id": userID, "card_id": cardID}).
		RunWith(runner).
		QueryContext(ctx)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		existing[id] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if closeErr := rows.Close(); closeErr != nil {
		log.Error().Err(closeErr).Msg("error closing rows")
	}

	keep := map[string]bool{}
	for _, c := range copies {
		if c.ID != "" {
			keep[c.ID] = true
		}
	}

	var toDelete []string
	for id := range existing {
		if !keep[id] {
			toDelete = append(toDelete, id)
		}
	}
	if len(toDelete) > 0 {
		if _, err := sq.Delete("owned_card_copies").Where(sq.Eq{"id": toDelete}).RunWith(runner).ExecContext(ctx); err != nil {
			return err
		}
	}

	for _, c := range copies {
		if c.ID == "" {
			id, err := NewUUIDv7()
			if err != nil {
				return err
			}
			_, err = sq.Insert("owned_card_copies").
				Columns("id", "user_id", "card_id", "price_paid_cents").
				Values(id, userID, cardID, c.PricePaidCents).
				RunWith(runner).
				ExecContext(ctx)
			if err != nil {
				return err
			}
			continue
		}

		if !existing[c.ID] {
			// Doesn't belong to this user/card - see doc comment above.
			continue
		}
		_, err := sq.Update("owned_card_copies").
			Set("price_paid_cents", c.PricePaidCents).
			Where(sq.Eq{"id": c.ID, "user_id": userID, "card_id": cardID}).
			RunWith(runner).
			ExecContext(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

// reconcileOwnedCardCopies adjusts the owned_card_copies rows for
// (userID, cardID) so their count matches quantity, inserting or deleting
// individual copy rows rather than rewriting the whole set - existing
// copies (and whatever price each already carries) are left alone except
// where noted below. This is what makes #108's real goal - each physical
// copy tracking its own price - actually reachable through the existing
// single quantity+price form: bumping quantity by one and entering that
// copy's price only ever touches the new copy, so an earlier copy's
// recorded price survives untouched.
//
// Deleted copies (quantity going down) are always the most-recently-added
// ones first - created_at DESC (id DESC only as a tiebreaker, since
// backfilled rows got a MySQL UUID() id, not a real UUIDv7 - see the
// backfill migration's own comment - so id ordering alone isn't safely
// comparable against rows this package inserts with NewUUIDv7). This is a
// real chronological LIFO, not an arbitrary one, but there's no
// principled better choice of *which* copies to remove; #108 itself flags
// that as an open question with no answer that doesn't lose some
// information, and a real per-copy "list of copies, edit each one" UI
// (also called out in #108 as its own follow-up) is what actually resolves
// it properly.
//
// If setPrice is true, pricePaidCents (nil included) is applied to
// whichever copy is now the most-recently-added survivor once the count
// above is settled - covers both "quantity went up, this is the new
// copy's price" and "quantity unchanged, I'm just correcting the price"
// with one rule. SetCardQuantity passes setPrice=false since it has no
// price of its own to give; the price of any newly-inserted copy in that
// path is simply left nil (matching SetCardQuantity's pre-#108 contract,
// which never touched price_paid_cents either).
func reconcileOwnedCardCopies(ctx context.Context, runner sq.BaseRunner, userID int, cardID string, quantity int, pricePaidCents *int, setPrice bool) error {
	var currentCount int
	err := sq.Select("COUNT(*)").
		From("owned_card_copies").
		Where(sq.Eq{"user_id": userID, "card_id": cardID}).
		RunWith(runner).
		QueryRowContext(ctx).
		Scan(&currentCount)
	if err != nil {
		return err
	}

	switch {
	case quantity > currentCount:
		toAdd := quantity - currentCount
		insert := sq.Insert("owned_card_copies").Columns("id", "user_id", "card_id")
		for i := 0; i < toAdd; i++ {
			id, err := NewUUIDv7()
			if err != nil {
				return err
			}
			insert = insert.Values(id, userID, cardID)
		}
		if _, err := insert.RunWith(runner).ExecContext(ctx); err != nil {
			return err
		}

	case quantity < currentCount:
		toRemove := currentCount - quantity
		rows, err := sq.Select("id").
			From("owned_card_copies").
			Where(sq.Eq{"user_id": userID, "card_id": cardID}).
			OrderBy("created_at DESC", "id DESC").
			Limit(uint64(toRemove)).
			RunWith(runner).
			QueryContext(ctx)
		if err != nil {
			return err
		}

		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if closeErr := rows.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing rows")
		}

		if len(ids) > 0 {
			if _, err := sq.Delete("owned_card_copies").Where(sq.Eq{"id": ids}).RunWith(runner).ExecContext(ctx); err != nil {
				return err
			}
		}
	}

	if !setPrice || quantity == 0 {
		return nil
	}

	_, err = sq.Update("owned_card_copies").
		Set("price_paid_cents", pricePaidCents).
		Where(sq.Eq{"user_id": userID, "card_id": cardID}).
		OrderBy("created_at DESC", "id DESC").
		Limit(1).
		RunWith(runner).
		ExecContext(ctx)
	return err
}

// ListOwnedCardsBySet returns userID's ownership for every card belonging
// to setID that has at least one owned_card_copies row - Quantity/
// PricePaidCents are a plain len()/sum() over Copies, and Copies itself is
// every one of that card's copies, oldest first (created_at, id - see
// reconcileOwnedCardCopies' doc comment for why id alone isn't safely
// comparable across backfilled vs. live rows), each with its own real id
// and price. Rows are fetched ungrouped and aggregated in Go rather than
// via SQL COUNT/SUM/GROUP BY (contrast GetOwnedCard, which only needs the
// aggregate for one card) specifically so this can return real per-copy
// identity for #108's editing UI without a second per-card round trip.
//
// Unlike before #108, a card explicitly set to quantity 0 no longer
// appears here at all - there's no physical row left to distinguish
// "explicitly marked not owned" from "never interacted with" once quantity
// is a COUNT(*) rather than a stored column (see OwnedCard's doc comment);
// the two are now the same state. Nothing observable currently depended on
// that distinction (OnboardCards.tsx's stepper defaults a missing entry to
// 0 the same as an explicit one), so this is a deliberate simplification,
// not an oversight.
func (p *Persist) ListOwnedCardsBySet(ctx context.Context, userID int, setID string) ([]CardQuantity, error) {
	rows, err := sq.Select("occ.id", "occ.card_id", "occ.price_paid_cents").
		From("owned_card_copies occ").
		Join("cards c ON c.id = occ.card_id").
		Where(sq.Eq{"occ.user_id": userID, "c.set_id": setID}).
		OrderBy("occ.card_id", "occ.created_at", "occ.id").
		RunWith(p.DB).
		QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing rows")
		}
	}()

	// order tracks first-seen card_id order (stable, since the query above
	// is itself ordered by card_id) - byCard alone (a plain map) wouldn't
	// preserve that for the []CardQuantity this returns.
	var order []string
	byCard := map[string]*CardQuantity{}
	for rows.Next() {
		var copyID, cardID string
		var priceCents sql.NullInt64
		if err := rows.Scan(&copyID, &cardID, &priceCents); err != nil {
			return nil, err
		}

		cq, ok := byCard[cardID]
		if !ok {
			cq = &CardQuantity{CardID: cardID}
			byCard[cardID] = cq
			order = append(order, cardID)
		}

		var price *int
		if priceCents.Valid {
			cents := int(priceCents.Int64)
			price = &cents
			if cq.PricePaidCents == nil {
				zero := 0
				cq.PricePaidCents = &zero
			}
			*cq.PricePaidCents += cents
		}
		cq.Copies = append(cq.Copies, CardCopy{ID: copyID, PricePaidCents: price})
		cq.Quantity++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	owned := make([]CardQuantity, 0, len(order))
	for _, cardID := range order {
		owned = append(owned, *byCard[cardID])
	}
	return owned, nil
}

// GetOwnedCard returns userID's ownership aggregate for cardID - Quantity
// is a COUNT(*) and PricePaidCents a SUM(price_paid_cents) over that card's
// owned_card_copies rows (see OwnedCard's doc comment). If no copies exist
// (the user has never interacted with this card's ownership, or explicitly
// owns zero - the two are indistinguishable post-#108), this returns a
// zero-quantity OwnedCard rather than an error - "not owned" is an
// ordinary state here, not an exceptional one. Unlike the old single-row
// SELECT this replaces, the aggregate query below always returns exactly
// one row regardless of match count, so there's no sql.ErrNoRows case to
// handle any more.
func (p *Persist) GetOwnedCard(ctx context.Context, userID int, cardID string) (OwnedCard, error) {
	row := sq.Select("COUNT(*)", "SUM(price_paid_cents)", "MAX(created_at)").
		From("owned_card_copies").
		Where(sq.Eq{"user_id": userID, "card_id": cardID}).
		RunWith(p.DB).
		QueryRowContext(ctx)

	var quantity int
	var priceSum sql.NullInt64
	var lastAdded sql.NullTime
	if err := row.Scan(&quantity, &priceSum, &lastAdded); err != nil {
		return OwnedCard{}, err
	}

	oc := OwnedCard{UserID: userID, CardID: cardID, Quantity: quantity}
	if priceSum.Valid {
		cents := int(priceSum.Int64)
		oc.PricePaidCents = &cents
	}
	if lastAdded.Valid {
		oc.UpdatedAt = lastAdded.Time
	}

	return oc, nil
}
