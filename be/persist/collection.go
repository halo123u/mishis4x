package persist

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
)

// ErrSetNotFound is returned by GetSet when no row matches.
var ErrSetNotFound = errors.New("set not found")

type Set struct {
	ID          string
	Name        string
	CardCount   int
	ReleaseDate *time.Time
	Status      string
	CreatedAt   time.Time
}

type Card struct {
	ID        string
	SetID     string
	Name      string
	Code      string
	Rarity    string
	CreatedAt time.Time
}

// CreateSet inserts a new set and returns its generated UUIDv7 ID.
func (p *Persist) CreateSet(ctx context.Context, name string, cardCount int, releaseDate *time.Time, status string) (string, error) {
	id, err := NewUUIDv7()
	if err != nil {
		return "", err
	}

	q := `
		INSERT INTO sets (id, name, card_count, release_date, status)
		VALUES (?, ?, ?, ?, ?);
	`
	if _, err := p.DB.ExecContext(ctx, q, id, name, cardCount, releaseDate, status); err != nil {
		return "", err
	}

	return id, nil
}

// GetSet looks up a set by ID. Returns ErrSetNotFound if no row matches.
func (p *Persist) GetSet(ctx context.Context, id string) (Set, error) {
	q := `
		SELECT id, name, card_count, release_date, status, created_at
		FROM sets
		WHERE id = ?;
	`
	var s Set
	var releaseDate sql.NullTime
	err := p.DB.QueryRowContext(ctx, q, id).Scan(&s.ID, &s.Name, &s.CardCount, &releaseDate, &s.Status, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Set{}, ErrSetNotFound
		}
		return Set{}, err
	}
	if releaseDate.Valid {
		s.ReleaseDate = &releaseDate.Time
	}

	return s, nil
}

// CreateCard inserts a new card belonging to setID and returns its
// generated UUIDv7 ID.
func (p *Persist) CreateCard(ctx context.Context, setID, name, code, rarity string) (string, error) {
	id, err := NewUUIDv7()
	if err != nil {
		return "", err
	}

	q := `
		INSERT INTO cards (id, set_id, name, code, rarity)
		VALUES (?, ?, ?, ?, ?);
	`
	if _, err := p.DB.ExecContext(ctx, q, id, setID, name, code, rarity); err != nil {
		return "", err
	}

	return id, nil
}

// ListCardsBySet returns every card belonging to setID, in insertion order
// (UUIDv7 IDs sort by creation time, so ORDER BY id doubles as ORDER BY
// created_at without needing a separate index).
func (p *Persist) ListCardsBySet(ctx context.Context, setID string) ([]Card, error) {
	q := `
		SELECT id, set_id, name, code, rarity, created_at
		FROM cards
		WHERE set_id = ?
		ORDER BY id;
	`
	rows, err := p.DB.QueryContext(ctx, q, setID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing rows")
		}
	}()

	var cards []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.SetID, &c.Name, &c.Code, &c.Rarity, &c.CreatedAt); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}

	return cards, rows.Err()
}
