package persist

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

// GetCardImage returns the stored image bytes, content type, and when it
// was last stored/replaced for cardID. Returns ErrCardImageNotFound if
// cardID has no image yet, rather than an empty result indistinguishable
// from a real (if unlikely) empty file.
//
// updatedAt exists so callers can serve a real Last-Modified header - see
// GetCardImage in handlers/collection.go, which uses it for conditional
// requests (a client that already has the current image gets a cheap 304
// on revalidation instead of the full bytes again, and a genuinely
// replaced image - see process-set re-imports - is detected correctly
// rather than just timing out of a blind cache window).
func (p *Persist) GetCardImage(ctx context.Context, cardID string) ([]byte, string, time.Time, error) {
	row := sq.Select("image", "content_type", "updated_at").
		From("card_images").
		Where(sq.Eq{"card_id": cardID}).
		RunWith(p.DB).
		QueryRowContext(ctx)

	var image []byte
	var contentType string
	var updatedAt time.Time
	if err := row.Scan(&image, &contentType, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", time.Time{}, ErrCardImageNotFound
		}
		return nil, "", time.Time{}, err
	}

	return image, contentType, updatedAt, nil
}
