package cmd

import (
	"context"

	"example.com/mishis4x/logger"
	"example.com/mishis4x/persist"
	"example.com/mishis4x/pricesync"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func init() {
	rootCMD.AddCommand(syncPricesCMD)
	syncPricesCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to connect to")
}

var syncPricesCMD = &cobra.Command{
	Use:   "sync-prices",
	Short: "Check every configured card_price_sources url and record fresh card_price_history rows",
	Long: `Fetches every distinct url in card_price_sources once, matches each card
that points at it against what's found there, and records the result in
card_price_history (updating that card's last_checked_at either way).

A manual one-shot command - be http also runs this same logic (see
pricesync.SyncAll) automatically on a schedule when ENABLE_PRICE_SYNC is
set, but this remains useful for an on-demand run without waiting for
that schedule.

Only "tcg_republic" is a supported source right now - anything else is
logged and skipped per-card, not fatal.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger.Init(env)

		db, err := persist.NewDB(env)
		if err != nil {
			log.Fatal().Err(err).Msg("error connecting to db")
		}
		defer func() {
			if closeErr := db.Close(); closeErr != nil {
				log.Error().Err(closeErr).Msg("error closing db connection")
			}
		}()
		p := &persist.Persist{DB: db}

		if _, err := pricesync.SyncAll(context.Background(), p); err != nil {
			log.Fatal().Err(err).Msg("sync-prices failed")
		}
	},
}
