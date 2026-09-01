package cmd

import (
	"context"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"

	"example.com/mishis4x/logger"
	"example.com/mishis4x/persist"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func init() {
	rootCMD.AddCommand(setPriceSourcesCMD)
	setPriceSourcesCMD.Flags().StringVarP(&setPriceSourcesName, "name", "n", "", "Set stored under the standard sets/<name>/ layout (see be/.gitignore) - reads sets/<name>/price_sources.csv and sets/<name>/set.json")
	setPriceSourcesCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to connect to")
}

var setPriceSourcesName string

var setPriceSourcesCMD = &cobra.Command{
	Use:   "set-price-sources",
	Short: "Load per-card price-check URLs (card_price_sources) for a set from a local CSV",
	Long: `Load per-card price-check URLs into card_price_sources for a set already
imported via process-set.

Reads sets/<name>/price_sources.csv - never committed, same standard
per-set layout process-set uses for catalog.csv/set.json/images (see
be/.gitignore). Expected header: code,source,url (column order doesn't
matter, extra columns are ignored).

source picks which parser the background price-sync job (and the
on-demand "check now" endpoint) applies to that row - e.g. "tcg_republic".
url is fully free-form, not reconstructed from any source-specific ID
template - point it at whatever page you actually want checked.

sets/<name>/set.json supplies the set's real name, used to resolve which
set's cards to match code against. Unlike process-set, this doesn't
create a set that doesn't exist yet - the set has to already be imported.

Upserts by card code, so re-running to fix a typo'd URL is safe: the
existing row's last_checked_at is left untouched rather than reset to
NULL, so a correction doesn't jump that card to the front of the sync
job's queue ahead of everything else's actual schedule.

A code with no matching card in the set is logged and skipped, not
fatal - the same tolerance process-set gives a malformed CSV row.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger.Init(env)

		if setPriceSourcesName == "" {
			log.Fatal().Msg("set-price-sources requires --name")
		}

		base := filepath.Join("sets", setPriceSourcesName)
		setPriceSources(filepath.Join(base, "price_sources.csv"), filepath.Join(base, "set.json"))
	},
}

func setPriceSources(file, setFile string) {
	meta, err := readSetMetadata(setFile)
	if err != nil {
		log.Fatal().Err(err).Str("file", setFile).Msg("error reading set metadata file")
	}

	f, err := os.Open(file)
	if err != nil {
		log.Fatal().Err(err).Str("file", file).Msg("error opening price sources file")
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing price sources file")
		}
	}()

	db, err := persist.NewDB(env)
	if err != nil {
		log.Fatal().Err(err).Msg("error connecting to db")
	}
	p := &persist.Persist{DB: db}
	ctx := context.Background()

	setID, err := p.GetSetIDByName(ctx, meta.Name)
	if err != nil {
		log.Fatal().Err(err).Str("set", meta.Name).Msg("error resolving set - has it been imported via process-set yet?")
	}

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		log.Fatal().Err(err).Msg("error reading CSV header")
	}
	col := make(map[string]int, len(header))
	for i, name := range header {
		col[name] = i
	}
	for _, required := range []string{"code", "source", "url"} {
		if _, ok := col[required]; !ok {
			log.Fatal().Str("column", required).Msg("price sources file is missing a required column")
		}
	}

	rows, err := r.ReadAll()
	if err != nil {
		log.Fatal().Err(err).Msg("error reading CSV rows")
	}

	var loaded, skipped int
	for _, row := range rows {
		code := row[col["code"]]
		source := row[col["source"]]
		url := row[col["url"]]

		cardID, err := p.GetCardIDByCode(ctx, setID, code)
		if err != nil {
			if errors.Is(err, persist.ErrCardNotFound) {
				log.Error().Str("code", code).Msg("no matching card for this code, skipping")
			} else {
				log.Error().Err(err).Str("code", code).Msg("error resolving card, skipping")
			}
			skipped++
			continue
		}

		if err := p.UpsertPriceSource(ctx, cardID, source, url); err != nil {
			log.Error().Err(err).Str("code", code).Msg("error upserting price source, skipping")
			skipped++
			continue
		}
		loaded++
	}

	log.Info().Int("loaded", loaded).Int("skipped", skipped).Str("set", meta.Name).Msg("set-price-sources finished")
}
