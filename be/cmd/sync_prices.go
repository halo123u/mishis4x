package cmd

import (
	"context"
	"time"

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

// syncPricesDelay is the pause between fetching each distinct price-source
// url - a few seconds apart is plenty polite given how few distinct urls
// there typically are (many cards share one url - see
// ListDistinctPriceSourceURLs's doc comment), this isn't 129 requests, it's
// one per listing page.
const syncPricesDelay = 2 * time.Second

var syncPricesCMD = &cobra.Command{
	Use:   "sync-prices",
	Short: "Check every configured card_price_sources url and record fresh card_price_history rows",
	Long: `Fetches every distinct url in card_price_sources once, matches each card
that points at it against what's found there, and records the result in
card_price_history (updating that card's last_checked_at either way - see
RecordPriceCheck's doc comment).

Currently a manual one-shot command, for a first, direct pass at this
rather than waiting on the background job/on-demand endpoint this is
meant to eventually feed - both would call the same underlying sync logic
this command does.

Only "tcg_republic" is a supported source right now - anything else is
logged and skipped per-card, not fatal.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger.Init(env)
		syncPrices()
	},
}

func syncPrices() {
	db, err := persist.NewDB(env)
	if err != nil {
		log.Fatal().Err(err).Msg("error connecting to db")
	}
	p := &persist.Persist{DB: db}
	ctx := context.Background()

	urls, err := p.ListDistinctPriceSourceURLs(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("error listing price source urls")
	}
	log.Info().Int("urls", len(urls)).Msg("sync-prices starting")

	var checked, matched, unmatched, errored int
	for i, url := range urls {
		cards, err := p.ListPriceSourcesForURL(ctx, url)
		if err != nil {
			log.Error().Err(err).Str("url", url).Msg("error listing cards for url, skipping")
			errored++
			continue
		}
		if len(cards) == 0 {
			continue
		}

		// Every card sharing a url is assumed to share the same source -
		// true for how set-price-sources populates rows today (one CSV,
		// one source column per set). Dispatching per-card rather than
		// per-url would only matter if that ever stopped holding.
		source := cards[0].Source
		if source != "tcg_republic" {
			log.Error().Str("source", source).Str("url", url).Msg("unsupported source, skipping")
			errored += len(cards)
			continue
		}

		items, err := pricesync.FetchTCGRepublicListing(ctx, url)
		if err != nil {
			log.Error().Err(err).Str("url", url).Msg("error fetching listing page, skipping")
			errored += len(cards)
			continue
		}

		for _, c := range cards {
			item, found := pricesync.MatchListingItem(items, c.Code)

			var priceCents *int
			if found {
				priceCents = &item.PriceCents
				matched++
			} else {
				unmatched++
			}

			if err := p.RecordPriceCheck(ctx, c.CardID, c.Source, priceCents); err != nil {
				log.Error().Err(err).Str("code", c.Code).Msg("error recording price check, skipping")
				errored++
				continue
			}
			checked++
		}

		log.Info().Str("url", url).Int("cards", len(cards)).Msg("checked url")

		if i < len(urls)-1 {
			time.Sleep(syncPricesDelay)
		}
	}

	log.Info().Int("checked", checked).Int("matched", matched).Int("unmatched", unmatched).Int("errored", errored).Msg("sync-prices finished")
}
