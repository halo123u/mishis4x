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
	UserID    int
	CardID    string
	Quantity  int
	UpdatedAt time.Time
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

// CardQuantity pairs a card with a quantity - the unit SetOwnedCards
// operates on in bulk, as opposed to SetCardQuantity's one-card-at-a-time
// form.
type CardQuantity struct {
	CardID   string
	Quantity int
}

// SetOwnedCards upserts quantity for every entry in cards in a single
// statement - the bulk form of SetCardQuantity, used by the onboarding
// flow's card-selection step, where a user submits many cards' ownership
// at once rather than one at a time. A nil/empty cards is a no-op, not an
// error - it never runs an empty INSERT.
func (p *Persist) SetOwnedCards(ctx context.Context, userID int, cards []CardQuantity) error {
	if len(cards) == 0 {
		return nil
	}

	insert := sq.Insert("owned_cards").
		Columns("user_id", "card_id", "quantity").
		Suffix("ON DUPLICATE KEY UPDATE quantity = VALUES(quantity)")
	for _, c := range cards {
		insert = insert.Values(userID, c.CardID, c.Quantity)
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
	rows, err := sq.Select("oc.card_id", "oc.quantity").
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
		if err := rows.Scan(&cq.CardID, &cq.Quantity); err != nil {
			return nil, err
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
	row := sq.Select("user_id", "card_id", "quantity", "updated_at").
		From("owned_cards").
		Where(sq.Eq{"user_id": userID, "card_id": cardID}).
		RunWith(p.DB).
		QueryRowContext(ctx)

	var oc OwnedCard
	err := row.Scan(&oc.UserID, &oc.CardID, &oc.Quantity, &oc.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OwnedCard{UserID: userID, CardID: cardID, Quantity: 0}, nil
		}
		return OwnedCard{}, err
	}

	return oc, nil
}
