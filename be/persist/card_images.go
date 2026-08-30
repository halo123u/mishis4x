package persist

import (
	"context"
	"database/sql"
	"errors"

	sq "github.com/Masterminds/squirrel"
)

// ErrCardImageNotFound is returned by GetCardImage when cardID has no
// stored image - a card missing an image is an ordinary, expected state
// (image coverage is populated incrementally, not required at import time),
// not a data-integrity problem.
var ErrCardImageNotFound = errors.New("card image not found")

// UpsertCardImage stores image (and its content type) for cardID, replacing
// whatever was stored before if this is a re-run - safe to call repeatedly
// for the same card, matching UpsertCard's own re-run safety.
func (p *Persist) UpsertCardImage(ctx context.Context, cardID string, image []byte, contentType string) error {
	_, err := sq.Insert("card_images").
		Columns("card_id", "image", "content_type").
		Values(cardID, image, contentType).
		Suffix("ON DUPLICATE KEY UPDATE image = VALUES(image), content_type = VALUES(content_type)").
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}

// GetCardImage returns the stored image bytes and content type for cardID.
// Returns ErrCardImageNotFound if cardID has no image yet, rather than an
// empty result indistinguishable from a real (if unlikely) empty file.
func (p *Persist) GetCardImage(ctx context.Context, cardID string) ([]byte, string, error) {
	row := sq.Select("image", "content_type").
		From("card_images").
		Where(sq.Eq{"card_id": cardID}).
		RunWith(p.DB).
		QueryRowContext(ctx)

	var image []byte
	var contentType string
	if err := row.Scan(&image, &contentType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", ErrCardImageNotFound
		}
		return nil, "", err
	}

	return image, contentType, nil
}
