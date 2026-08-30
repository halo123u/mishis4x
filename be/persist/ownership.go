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
// again) and every owned_cards row for one of setID's cards (so re-adding
// the set later starts from a clean slate instead of resurrecting old
// ownership data via the editor form's pre-fill). A no-op, not an error,
// if userID never onboarded setID - same idempotent shape as SetOwnedSet.
func (p *Persist) DeleteOwnedSet(ctx context.Context, userID int, setID string) error {
	_, err := sq.Delete("owned_cards").
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

// SetCardQuantity upserts how many copies of cardID userID owns. A
// quantity of 0 is a valid state (explicitly marked "don't own this"),
// distinct from no row existing at all (never interacted with).
func (p *Persist) SetCardQuantity(ctx context.Context, userID int, cardID string, quantity int) error {
	_, err := sq.Insert("owned_cards").
		Columns("user_id", "card_id", "quantity").
		Values(userID, cardID, quantity).
		Suffix("ON DUPLICATE KEY UPDATE quantity = VALUES(quantity)").
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}

// CardQuantity pairs a card with a quantity (and optionally what it cost) -
// the unit SetOwnedCards operates on in bulk, as opposed to
// SetCardQuantity's one-card-at-a-time form. PricePaidCents is nil when
// unknown, distinct from a real $0 - and like Quantity, whatever's passed
// here fully replaces the stored value, nil included. A caller that wants
// to leave an already-recorded price alone has to read it first and pass
// it back, the same diff-then-resubmit shape the onboard/edit form already
// uses for quantity itself (see OnboardCards.tsx's handleSave).
type CardQuantity struct {
	CardID         string
	Quantity       int
	PricePaidCents *int
}

// SetOwnedCards upserts quantity and price for every entry in cards in a
// single statement - the bulk form of SetCardQuantity, used by the
// onboarding flow's card-selection step, where a user submits many cards'
// ownership at once rather than one at a time. A nil/empty cards is a
// no-op, not an error - it never runs an empty INSERT.
func (p *Persist) SetOwnedCards(ctx context.Context, userID int, cards []CardQuantity) error {
	if len(cards) == 0 {
		return nil
	}

	insert := sq.Insert("owned_cards").
		Columns("user_id", "card_id", "quantity", "price_paid_cents").
		Suffix("ON DUPLICATE KEY UPDATE quantity = VALUES(quantity), price_paid_cents = VALUES(price_paid_cents)")
	for _, c := range cards {
		insert = insert.Values(userID, c.CardID, c.Quantity, c.PricePaidCents)
	}

	_, err := insert.RunWith(p.DB).ExecContext(ctx)
	return err
}

// ListOwnedCardsBySet returns userID's ownership rows for every card
// belonging to setID that userID has an owned_cards row for - including
// explicit zero-quantity rows, since callers (the set-editor form's
// pre-fill, in particular) need to tell "explicitly marked not owned" apart
// from "never interacted" the same way GetOwnedCard does for a single card.
// A card with no row at all simply doesn't appear in the result.
func (p *Persist) ListOwnedCardsBySet(ctx context.Context, userID int, setID string) ([]CardQuantity, error) {
	rows, err := sq.Select("oc.card_id", "oc.quantity", "oc.price_paid_cents").
		From("owned_cards oc").
		Join("cards c ON c.id = oc.card_id").
		Where(sq.Eq{"oc.user_id": userID, "c.set_id": setID}).
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

	var owned []CardQuantity
	for rows.Next() {
		var cq CardQuantity
		var priceCents sql.NullInt64
		if err := rows.Scan(&cq.CardID, &cq.Quantity, &priceCents); err != nil {
			return nil, err
		}
		if priceCents.Valid {
			cents := int(priceCents.Int64)
			cq.PricePaidCents = &cents
		}
		owned = append(owned, cq)
	}

	return owned, rows.Err()
}

// GetOwnedCard returns userID's ownership row for cardID. If no row exists
// yet (the user has never interacted with this card's ownership), it
// returns a zero-quantity OwnedCard rather than an error - "not owned" is
// an ordinary state here, not an exceptional one.
func (p *Persist) GetOwnedCard(ctx context.Context, userID int, cardID string) (OwnedCard, error) {
	row := sq.Select("user_id", "card_id", "quantity", "price_paid_cents", "updated_at").
		From("owned_cards").
		Where(sq.Eq{"user_id": userID, "card_id": cardID}).
		RunWith(p.DB).
		QueryRowContext(ctx)

	var oc OwnedCard
	var priceCents sql.NullInt64
	err := row.Scan(&oc.UserID, &oc.CardID, &oc.Quantity, &priceCents, &oc.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OwnedCard{UserID: userID, CardID: cardID, Quantity: 0}, nil
		}
		return OwnedCard{}, err
	}
	if priceCents.Valid {
		cents := int(priceCents.Int64)
		oc.PricePaidCents = &cents
	}

	return oc, nil
}
