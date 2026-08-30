package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/mishis4x/logger"
	"example.com/mishis4x/persist"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func init() {
	rootCMD.AddCommand(processSetCMD)
	processSetCMD.Flags().StringVarP(&processSetFile, "file", "f", "", "CSV file to import")
	processSetCMD.Flags().StringVarP(&processSetMetaFile, "set-file", "s", "", "Optional JSON file with the set's real metadata (name, card_count, release_date, status)")
	processSetCMD.Flags().BoolVarP(&processSetRefresh, "refresh", "r", false, "Wipe the set named in --set-file before reimporting, instead of upserting on top of it (requires --set-file)")
	processSetCMD.Flags().StringVarP(&processSetImagesDir, "images-dir", "i", "", "Optional local directory of card images to import alongside the CSV - never committed, see the flag's help in --help output for the filename convention")
	processSetCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to connect to")
}

var processSetFile string
var processSetMetaFile string
var processSetRefresh bool
var processSetImagesDir string

// imageExtensions are tried in this order for each card code - first match
// wins. Deliberately not configurable; these cover every format a card
// scan/photo would realistically show up in.
var imageExtensions = []string{".jpg", ".jpeg", ".png", ".webp", ".gif"}

var processSetCMD = &cobra.Command{
	Use:   "process-set",
	Short: "Import a card-catalog CSV (issue #68) into sets/cards",
	Long: `Import a card-catalog CSV into sets/cards.

Expected header: set_name,card_number,name_japanese,rarity (column order
doesn't matter, extra columns are ignored). Upserts into cards, so
re-running against an updated CSV is safe rather than additive; sets are
looked up/created by name as they're encountered.

--set-file optionally gives the set real metadata instead of the
placeholder card_count=0/release_date=nil/status="pending" that a
name-only lookup would otherwise fall back to. JSON shape:

  {"name": "Brown Dust 2", "card_count": 100, "release_date": "2026-07-03", "status": "active"}

release_date is "YYYY-MM-DD" and optional (omit or leave "" for no date).

--refresh removes cards under the set named in --set-file that AREN'T in
this CSV, rather than upserting on top of what's there with old, now-gone
codes left stranded behind - the set's own id/metadata are left alone,
and any code that still appears in the new CSV is left completely alone
too (updated in place via the ordinary upsert, same as without --refresh -
its id, ownership, and image all survive untouched). Requires --set-file,
since a name is needed to know which set to reconcile before anything
else runs. Use this when card codes in the CSV changed shape (e.g.
normalizing "001S" to "BRD/W139-001S") - UpsertCard's (set_id, code) match
won't recognize the old rows as the same cards, so re-running without
--refresh would leave old and new duplicates side by side instead of
replacing them.

Only codes that are genuinely gone from the new CSV get deleted - if one
of those specific cards has real ownership data (someone's tracking it),
that delete fails loudly rather than silently discarding it, the same
protection real card deletion always has. A code that persists across
old and new CSVs is never a candidate for deletion in the first place, so
its ownership/image is never at risk regardless of what else in the set
is or isn't owned - --refresh isn't an all-or-nothing gate on the whole
set the way a blanket wipe-then-reimport would be.

--images-dir optionally attaches a reference image to each card as it's
imported, read from local files - never committed to the repo (same
convention as be/db/misc). A card's file is matched by its code with "/"
replaced by "_" (cards.code often contains a literal "/", e.g.
"BRD/W139-001S" -> looks for "BRD_W139-001S.jpg", trying .jpg/.jpeg/.png/
.webp/.gif in that order). A card with no matching file simply gets no
image - not an error, since image coverage is expected to fill in
incrementally, not all at once. A card whose code is unchanged across a
--refresh keeps its existing image too (its id never changes); only a
code that's actually new needs --images-dir to supply one.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger.Init(env)
		processSet(processSetFile, processSetMetaFile, processSetImagesDir, processSetRefresh)
	},
}

// setMetadata is the shape expected in --set-file. release_date is a plain
// "YYYY-MM-DD" string (not RFC3339) since it's meant to be hand-written.
type setMetadata struct {
	Name        string `json:"name"`
	CardCount   int    `json:"card_count"`
	ReleaseDate string `json:"release_date"`
	Status      string `json:"status"`
}

func readSetMetadata(file string) (setMetadata, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return setMetadata{}, err
	}

	var meta setMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return setMetadata{}, err
	}

	return meta, nil
}

func processSet(file, setFile, imagesDir string, refresh bool) {
	if file == "" {
		log.Fatal().Msg("process-set requires --file")
	}
	if refresh && setFile == "" {
		log.Fatal().Msg("--refresh requires --set-file (need a name to know what to wipe)")
	}

	f, err := os.Open(file)
	if err != nil {
		log.Fatal().Err(err).Str("file", file).Msg("error opening import file")
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing import file")
		}
	}()

	db, err := persist.NewDB(env)
	if err != nil {
		log.Fatal().Err(err).Msg("error connecting to db")
	}
	p := &persist.Persist{DB: db}
	ctx := context.Background()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		log.Fatal().Err(err).Msg("error reading CSV header")
	}
	col := make(map[string]int, len(header))
	for i, name := range header {
		col[name] = i
	}
	for _, required := range []string{"set_name", "card_number", "name_japanese", "rarity"} {
		if _, ok := col[required]; !ok {
			log.Fatal().Str("column", required).Msg("import file is missing a required column")
		}
	}

	// Buffered up front, rather than streamed row-by-row, so --refresh
	// knows every code the new CSV contains for its target set *before*
	// touching the database - it needs that full picture to delete only
	// the codes that are genuinely gone, not everything.
	rows, err := r.ReadAll()
	if err != nil {
		log.Fatal().Err(err).Msg("error reading CSV rows")
	}

	// Cache set_name -> set_id within this run so a CSV with many rows for
	// the same set doesn't hit a lookup on every row. Seeded up front from
	// --set-file, if given, so its real metadata is what CSV rows for that
	// set resolve to rather than a fresh placeholder.
	setIDs := make(map[string]string)
	if setFile != "" {
		meta, err := readSetMetadata(setFile)
		if err != nil {
			log.Fatal().Err(err).Str("file", setFile).Msg("error reading set metadata file")
		}

		var releaseDate *time.Time
		if meta.ReleaseDate != "" {
			parsed, err := time.Parse("2006-01-02", meta.ReleaseDate)
			if err != nil {
				log.Fatal().Err(err).Str("release_date", meta.ReleaseDate).Msg("release_date must be YYYY-MM-DD")
			}
			releaseDate = &parsed
		}

		// Resolve/create the set BEFORE reconciling cards, so --refresh
		// works against the set's existing, stable id rather than
		// deleting the set row itself and forcing UpsertSetMetadata down
		// its create-fresh path - a set's id needs to stay stable across
		// refreshes (anything already linking to it - a bookmarked
		// /collection/{id} URL, owned_sets rows - breaks otherwise).
		setID, err := p.UpsertSetMetadata(ctx, meta.Name, meta.CardCount, releaseDate, meta.Status)
		if err != nil {
			log.Fatal().Err(err).Str("set", meta.Name).Msg("error applying set metadata")
		}
		log.Info().Str("set", meta.Name).Str("id", setID).Msg("applied set metadata")

		if refresh {
			var keepCodes []string
			for _, row := range rows {
				if row[col["set_name"]] == meta.Name {
					keepCodes = append(keepCodes, row[col["card_number"]])
				}
			}
			if len(keepCodes) == 0 {
				log.Fatal().Str("set", meta.Name).Msg("--refresh found no rows for this set in the CSV - refusing to wipe the whole set")
			}
			if err := p.DeleteCardsForSetExceptCodes(ctx, setID, keepCodes); err != nil {
				log.Fatal().Err(err).Str("set", meta.Name).Msg("error reconciling cards for --refresh")
			}
			log.Info().Str("set", meta.Name).Str("id", setID).Int("keptCodes", len(keepCodes)).Msg("reconciled cards for --refresh")
		}

		setIDs[meta.Name] = setID
	}

	var imported, skipped, imagesAttached int
	for _, row := range rows {
		setName := row[col["set_name"]]
		setID, ok := setIDs[setName]
		if !ok {
			var err error
			setID, err = p.GetOrCreateSetByName(ctx, setName)
			if err != nil {
				log.Fatal().Err(err).Str("set", setName).Msg("error resolving set")
			}
			setIDs[setName] = setID
		}

		// A single malformed row (e.g. a rarity value too long for the
		// column - real-world scraped/hand-edited CSVs aren't guaranteed
		// clean) shouldn't abort the whole import; log it and move on so
		// the rest of the file still gets imported.
		code := row[col["card_number"]]
		cardID, err := p.UpsertCard(ctx, setID, row[col["name_japanese"]], code, row[col["rarity"]])
		if err != nil {
			log.Error().Err(err).Str("code", code).Msg("error upserting card, skipping row")
			skipped++
			continue
		}
		imported++

		if imagesDir != "" && attachCardImage(ctx, p, imagesDir, cardID, code) {
			imagesAttached++
		}
	}

	log.Info().Int("cards", imported).Int("skipped", skipped).Int("images", imagesAttached).Int("sets", len(setIDs)).Msg("process-set finished")
}

// attachCardImage looks for a local image file matching code in imagesDir
// and, if found, stores it against cardID. Returns false (not an error -
// logged instead, same tolerance as a malformed CSV row) both when no
// matching file exists at all (the ordinary, expected case for a card
// whose image hasn't been sourced yet) and when a matching file exists but
// couldn't be read.
func attachCardImage(ctx context.Context, p *persist.Persist, imagesDir, cardID, code string) bool {
	path, found := findImageFile(imagesDir, code)
	if !found {
		return false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Error().Err(err).Str("code", code).Str("path", path).Msg("error reading card image, skipping")
		return false
	}

	contentType := http.DetectContentType(data)
	if err := p.UpsertCardImage(ctx, cardID, data, contentType); err != nil {
		log.Error().Err(err).Str("code", code).Msg("error storing card image, skipping")
		return false
	}

	return true
}

// findImageFile looks for a file named after code (with "/" replaced by
// "_", since a literal "/" in code would otherwise be read as a
// subdirectory) in dir, trying each of imageExtensions in order. Returns
// ("", false) if none exist - not an error, just no image for this card
// yet.
func findImageFile(dir, code string) (string, bool) {
	base := strings.ReplaceAll(code, "/", "_")
	for _, ext := range imageExtensions {
		path := filepath.Join(dir, base+ext)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}
