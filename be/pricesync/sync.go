package pricesync

import (
	"context"
	"time"

	"example.com/mishis4x/persist"
	"github.com/rs/zerolog/log"
)

// SyncDelay is the pause between fetching each distinct price-source url -
// a few seconds apart is plenty polite given how few distinct urls there
// typically are (many cards share one url - see
// persist.ListDistinctPriceSourceURLs's doc comment), this isn't 129
// requests, it's one per listing page.
const SyncDelay = 2 * time.Second

// Stats summarizes one SyncAll pass.
type Stats struct {
	Checked   int
	Matched   int
	Unmatched int
	Errored   int
}

// SyncAll fetches every distinct card_price_sources url once and records
// the result for every card pointing at it - shared by both the
// sync-prices CLI command and the background ticker started alongside
// be http (cmd/http.go), so there's exactly one code path that actually
// does a scrape+write, not two divergent copies.
//
// Respects ctx cancellation between urls (not mid-fetch) so a shutdown
// request doesn't have to wait out the full pacing delay - see
// cmd/http.go's use of this during graceful shutdown.
//
// Currently only "tcg_republic" is a supported source - anything else is
// logged and skipped per-card, not fatal, same tolerance a malformed CSV
// row gets elsewhere in this codebase.
func SyncAll(ctx context.Context, p *persist.Persist) (Stats, error) {
	var stats Stats

	urls, err := p.ListDistinctPriceSourceURLs(ctx)
	if err != nil {
		return stats, err
	}
	log.Info().Int("urls", len(urls)).Msg("price sync starting")

	for i, url := range urls {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}

		cards, err := p.ListPriceSourcesForURL(ctx, url)
		if err != nil {
			log.Error().Err(err).Str("url", url).Msg("error listing cards for url, skipping")
			stats.Errored++
			continue
		}
		if len(cards) == 0 {
			continue
		}

		// Every card sharing a url is assumed to share the same source -
		// true for how set-price-sources populates rows today (one CSV,
		// one source column per set).
		source := cards[0].Source
		if source != "tcg_republic" {
			log.Error().Str("source", source).Str("url", url).Msg("unsupported source, skipping")
			stats.Errored += len(cards)
			continue
		}

		items, err := FetchTCGRepublicListing(ctx, url)
		if err != nil {
			log.Error().Err(err).Str("url", url).Msg("error fetching listing page, skipping")
			stats.Errored += len(cards)
			continue
		}

		for _, c := range cards {
			item, found := MatchListingItem(items, c.Code)

			var priceCents *int
			if found {
				priceCents = &item.PriceCents
				stats.Matched++
			} else {
				stats.Unmatched++
			}

			if err := p.RecordPriceCheck(ctx, c.CardID, c.Source, priceCents); err != nil {
				log.Error().Err(err).Str("code", c.Code).Msg("error recording price check, skipping")
				stats.Errored++
				continue
			}
			stats.Checked++
		}

		log.Info().Str("url", url).Int("cards", len(cards)).Msg("checked url")

		if i < len(urls)-1 {
			select {
			case <-ctx.Done():
				return stats, ctx.Err()
			case <-time.After(SyncDelay):
			}
		}
	}

	log.Info().
		Int("checked", stats.Checked).
		Int("matched", stats.Matched).
		Int("unmatched", stats.Unmatched).
		Int("errored", stats.Errored).
		Msg("price sync finished")

	return stats, nil
}
