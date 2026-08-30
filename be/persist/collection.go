package persist

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"sort"
	"strconv"
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

// getSetIDByName looks up a set's id by name, returning ErrSetNotFound
// (the same sentinel GetSet uses) if no row matches - shared by every
// by-name operation below instead of each repeating the same SELECT.
//
// sets.name has no unique constraint (see the add_sets_table migration),
// so callers building a check-then-write on top of this aren't atomic - a
// real race is possible under concurrent callers, but every current
// caller is a one-at-a-time CLI invocation, not a web handler.
func (p *Persist) getSetIDByName(ctx context.Context, name string) (string, error) {
	row := sq.Select("id").
		From("sets").
		Where(sq.Eq{"name": name}).
		RunWith(p.DB).
		QueryRowContext(ctx)

	var id string
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSetNotFound
		}
		return "", err
	}

	return id, nil
}

// GetOrCreateSetByName looks up a set by name, creating it (with a
// placeholder card_count/status) if no row matches yet. Used by the
// process-set job, where the CSV only knows a set's name, not its ID.
func (p *Persist) GetOrCreateSetByName(ctx context.Context, name string) (string, error) {
	id, err := p.getSetIDByName(ctx, name)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, ErrSetNotFound) {
		return "", err
	}

	return p.CreateSet(ctx, name, 0, nil, "pending")
}

// UpsertSetMetadata looks up a set by name and updates its
// card_count/release_date/status in place, or creates it with those values
// if no row matches yet. Distinct from GetOrCreateSetByName, which only
// ever fills in placeholders - this is for when the caller actually has
// real metadata (e.g. the process-set job's optional --set-file) to apply,
// including correcting a set that already exists as a placeholder.
func (p *Persist) UpsertSetMetadata(ctx context.Context, name string, cardCount int, releaseDate *time.Time, status string) (string, error) {
	id, err := p.getSetIDByName(ctx, name)
	if err == nil {
		_, err = sq.Update("sets").
			Set("card_count", cardCount).
			Set("release_date", releaseDate).
			Set("status", status).
			Where(sq.Eq{"id": id}).
			RunWith(p.DB).
			ExecContext(ctx)
		return id, err
	}
	if !errors.Is(err, ErrSetNotFound) {
		return "", err
	}

	return p.CreateSet(ctx, name, cardCount, releaseDate, status)
}

// DeleteCardsForSet deletes every card belonging to setID, without
// touching the set row itself - the set's own id/name/metadata are left
// completely alone. See process-set's --refresh (cmd/jobs.go) for why
// this exists: card codes are the natural key process-set matches on, but
// that only works if a code stays spelled the same across runs.
// Renumbering/normalizing codes in the source CSV means UpsertCard's
// (set_id, code) match silently stops finding the old rows and inserts
// fresh duplicates instead of updating them - there's no database-
// generated id in the CSV to match on instead, and hand-adding one to a
// file you're regenerating from a scrape/spreadsheet isn't practical.
// Wiping the set's cards and reimporting from scratch sidesteps that
// entirely, at the cost of losing anything not represented in the CSV.
//
// Deliberately does NOT cascade into owned_cards - if any ownership rows
// reference these cards, the DELETE below hits their foreign key and
// fails loudly instead of silently orphaning a user's ownership data.
// That's intentional: --refresh is catalog-only tooling, not something
// that should be able to quietly wipe what a real user owns.
func (p *Persist) DeleteCardsForSet(ctx context.Context, setID string) error {
	_, err := sq.Delete("cards").Where(sq.Eq{"set_id": setID}).RunWith(p.DB).ExecContext(ctx)
	return err
}

// DeleteCardsForSetExceptCodes deletes every card in setID whose code isn't
// in keepCodes - the surgical counterpart to DeleteCardsForSet, used by
// process-set --refresh so re-running against an updated CSV only touches
// codes that actually disappeared, not the whole set. This matters because
// the FK check on owned_cards/card_images runs across the whole DELETE
// statement at once: DeleteCardsForSet fails if *any* card anywhere in the
// set is owned, even one whose code isn't changing at all, while this only
// fails if a card that's genuinely being removed (its code truly isn't in
// the new CSV) happens to be owned - a real, unavoidable conflict, not a
// side effect of cards that didn't need to move in the first place.
// keepCodes must be non-empty - an empty list would delete the entire set,
// which isn't a real CSV import (an empty CSV for the target set is
// almost certainly a mistake, not an intentional wipe).
func (p *Persist) DeleteCardsForSetExceptCodes(ctx context.Context, setID string, keepCodes []string) error {
	if len(keepCodes) == 0 {
		return errors.New("keepCodes must not be empty")
	}

	_, err := sq.Delete("cards").
		Where(sq.Eq{"set_id": setID}).
		Where(sq.NotEq{"code": keepCodes}).
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}

// DeleteSetCascade deletes a set's cards, then the set row itself, by
// name - a full removal, unlike DeleteCardsForSet, which deliberately
// preserves the set's own id/identity. A no-op (nil error) if no set with
// that name exists yet. Not used by process-set's --refresh (that only
// needs DeleteCardsForSet, to avoid churning the set's id on every
// refresh) - this is for genuinely retiring a set altogether.
func (p *Persist) DeleteSetCascade(ctx context.Context, name string) error {
	id, err := p.getSetIDByName(ctx, name)
	if errors.Is(err, ErrSetNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := p.DeleteCardsForSet(ctx, id); err != nil {
		return err
	}
	_, err = sq.Delete("sets").Where(sq.Eq{"id": id}).RunWith(p.DB).ExecContext(ctx)
	return err
}

// UpsertCard inserts a card, or updates its name/rarity in place if one
// already exists for the same (set_id, code) - the catalog importer's whole
// point is being safe to re-run against an updated CSV without duplicating
// rows. The existing row's id is left untouched on update. Returns the
// card's id either way - on a duplicate-key update the id generated below
// is never actually used (the existing row's original id wins), so it's
// looked up explicitly afterward rather than assumed, for callers (e.g.
// process-set attaching an image) that need the real id.
func (p *Persist) UpsertCard(ctx context.Context, setID, name, code, rarity string) (string, error) {
	id, err := NewUUIDv7()
	if err != nil {
		return "", err
	}

	_, err = sq.Insert("cards").
		Columns("id", "set_id", "name", "code", "rarity").
		Values(id, setID, name, code, rarity).
		Suffix("ON DUPLICATE KEY UPDATE name = VALUES(name), rarity = VALUES(rarity)").
		RunWith(p.DB).
		ExecContext(ctx)
	if err != nil {
		return "", err
	}

	return p.getCardIDByCode(ctx, setID, code)
}

// getCardIDByCode looks up a card's id by (set_id, code) - the natural key
// UpsertCard matches on, but not something callers can assume they already
// know the id for (that's the whole reason UpsertCard exists instead of a
// plain insert).
func (p *Persist) getCardIDByCode(ctx context.Context, setID, code string) (string, error) {
	row := sq.Select("id").
		From("cards").
		Where(sq.Eq{"set_id": setID, "code": code}).
		RunWith(p.DB).
		QueryRowContext(ctx)

	var id string
	if err := row.Scan(&id); err != nil {
		return "", err
	}

	return id, nil
}

// codePattern splits a card code into everything before its trailing
// number+letters, that trailing number, and its trailing letter suffix
// (the rarity-variant marker: "S", "SP", "SSP", "EX", ...). The prefix
// group is deliberately unanchored/non-greedy rather than restricted to
// letters-only, so this matches both a bare code ("001S") and one that
// still carries a full set code ("BRD/W139-001S", "T11S" for a trial
// deck) - only the trailing digit run followed by trailing letters has to
// exist, whatever comes before it becomes the prefix group as-is.
var codePattern = regexp.MustCompile(`^(.*?)(\d+)([A-Za-z]*)$`)

// starPattern pulls a star count out of rarity text like "SR 2-star" -
// only "S"-suffix cards' rarity happens to carry this; every other
// suffix's rarity is a flat label ("SP", "SSP", "SEC", ...) with no star
// to extract, which is fine, see cardSuffixRank's doc comment.
var starPattern = regexp.MustCompile(`(\d)-star`)

// cardSuffixRank orders a code's letter suffix into display groups: base
// "S" cards first, then their signed "SP" parallels, then rarer chase
// tiers, roughly in ascending scarcity. An unrecognized suffix sorts after
// all of these (alphabetically among themselves via the raw suffix
// string, handled by the caller) rather than erroring - this is a display
// nicety, not something that should ever break card listing over a code
// shape nobody's seen yet.
var cardSuffixRank = map[string]int{
	"S":   0,
	"SP":  1,
	"SSP": 2,
	"EX":  3,
	"SEC": 3, // interchangeable with EX in this set's data
	"R":   4,
	"RRR": 5,
	"CR":  6,
	"AGR": 7,
	"A":   7, // e.g. "098A" for an AGR-rarity card
	"TDP": 8,
}

// cardSortKey extracts (prefix, suffix rank, star tier, numeric code) from
// c for sortCardsForDisplay. A code that doesn't match codePattern at all
// (never seen in practice, but the format isn't enforced at the DB level)
// falls back to sorting by its raw string after everything recognized,
// rather than erroring.
func cardSortKey(c Card) (prefix string, suffixRank int, star int, num int) {
	m := codePattern.FindStringSubmatch(c.Code)
	if m == nil {
		return c.Code, len(cardSuffixRank) + 1, 0, 0
	}

	prefix = m[1]
	num, _ = strconv.Atoi(m[2])
	suffix := m[3]

	if r, ok := cardSuffixRank[suffix]; ok {
		suffixRank = r
	} else {
		suffixRank = len(cardSuffixRank) + 1
	}

	// Star tier only meaningfully applies to base "S" cards - other
	// suffixes' rarity text doesn't carry a star count, so this stays 0
	// for them, which has no effect since they don't share a suffixRank
	// with any "S" row to be compared against on this key.
	if suffix == "S" {
		if sm := starPattern.FindStringSubmatch(c.Rarity); sm != nil {
			star, _ = strconv.Atoi(sm[1])
		}
	}

	return prefix, suffixRank, star, num
}

// sortCardsForDisplay sorts cards for how they're actually meant to be
// browsed: grouped by rarity-variant suffix (all base "S" cards, then all
// "SP" signed parallels, then rarer chase tiers), with base "S" cards
// further grouped by star tier (all 1-star, then all 2-star, then all
// 3-star) before falling back to plain numeric order. This has to happen
// in Go rather than a SQL ORDER BY - the star tier lives inside a
// free-text rarity column ("SR 2-star") that's only consistently
// parseable for the "S" suffix, and expressing "group by suffix, but only
// sub-group by a value parsed out of a different column for one specific
// suffix" as SQL would mean fragile string-matching CASE expressions
// instead of one small, testable Go function.
func sortCardsForDisplay(cards []Card) {
	sort.SliceStable(cards, func(i, j int) bool {
		pi, ri, si, ni := cardSortKey(cards[i])
		pj, rj, sj, nj := cardSortKey(cards[j])
		if pi != pj {
			return pi < pj
		}
		if ri != rj {
			return ri < rj
		}
		if si != sj {
			return si < sj
		}
		if ni != nj {
			return ni < nj
		}
		return cards[i].Code < cards[j].Code
	})
}

// ListCardsBySet returns every card belonging to setID, sorted per
// sortCardsForDisplay's doc comment.
func (p *Persist) ListCardsBySet(ctx context.Context, setID string) ([]Card, error) {
	rows, err := sq.Select("id", "set_id", "name", "code", "rarity", "created_at").
		From("cards").
		Where(sq.Eq{"set_id": setID}).
		OrderBy("code").
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sortCardsForDisplay(cards)
	return cards, nil
}
