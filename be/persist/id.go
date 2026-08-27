package persist

import "github.com/google/uuid"

// NewUUIDv7 generates a new UUID version 7 for use as a primary key on
// tables that deliberately use non-sequential, timestamp-sortable IDs
// instead of AUTO_INCREMENT (sets, cards - see issue #75). Unlike a fully
// random UUIDv4, a v7 ID embeds a millisecond timestamp prefix, so IDs
// generated later always sort after ones generated earlier - inserts stay
// append-only in the InnoDB B-tree instead of landing at random points and
// fragmenting it.
func NewUUIDv7() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
