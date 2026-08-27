package persist

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
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

	_, err = sq.Insert("sets").
		Columns("id", "name", "card_count", "release_date", "status").
		Values(id, name, cardCount, releaseDate, status).
		RunWith(p.DB).
		ExecContext(ctx)
	if err != nil {
		return "", err
	}

	return id, nil
}

// GetSet looks up a set by ID. Returns ErrSetNotFound if no row matches.
func (p *Persist) GetSet(ctx context.Context, id string) (Set, error) {
	row := sq.Select("id", "name", "card_count", "release_date", "status", "created_at").
		From("sets").
		Where(sq.Eq{"id": id}).
		RunWith(p.DB).
		QueryRowContext(ctx)

	var s Set
	var releaseDate sql.NullTime
	err := row.Scan(&s.ID, &s.Name, &s.CardCount, &releaseDate, &s.Status, &s.CreatedAt)
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

// ListSets returns every set, in insertion order (see ListCardsBySet's doc
// comment for why ORDER BY id doubles as creation order for UUIDv7 keys).
func (p *Persist) ListSets(ctx context.Context) ([]Set, error) {
	rows, err := sq.Select("id", "name", "card_count", "release_date", "status", "created_at").
		From("sets").
		OrderBy("id").
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

// CreateCard inserts a new card belonging to setID and returns its
// generated UUIDv7 ID.
func (p *Persist) CreateCard(ctx context.Context, setID, name, code, rarity string) (string, error) {
	id, err := NewUUIDv7()
	if err != nil {
		return "", err
	}

	_, err = sq.Insert("cards").
		Columns("id", "set_id", "name", "code", "rarity").
		Values(id, setID, name, code, rarity).
		RunWith(p.DB).
		ExecContext(ctx)
	if err != nil {
		return "", err
	}

	return id, nil
}

// ListCardsBySet returns every card belonging to setID, in insertion order
// (UUIDv7 IDs sort by creation time, so ORDER BY id doubles as ORDER BY
// created_at without needing a separate index).
func (p *Persist) ListCardsBySet(ctx context.Context, setID string) ([]Card, error) {
	rows, err := sq.Select("id", "set_id", "name", "code", "rarity", "created_at").
		From("cards").
		Where(sq.Eq{"set_id": setID}).
		OrderBy("id").
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
