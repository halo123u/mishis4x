// Package pricesync scrapes card prices from external sale pages (there's
// no API access for any of these sources - eBay's is pending approval, TCG
// Republic doesn't offer one at all) and records them into
// card_price_history. See be/db/migrations/up/20260831_12_add_card_price_sources_table.sql
// and _13_add_card_price_history_table.sql for the schema this feeds.
package pricesync

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// ListingItem is one product parsed off a TCG Republic category listing
// page. Text is the product's raw alt/caption text (e.g. "ビーチの天使
// テレーゼ BRD/W139-075S SR Foil & Stamped") rather than an extracted code -
// this package doesn't know or assume any particular game's code format,
// so matching a specific card against these items (via MatchListingItem)
// is the caller's job, done with codes it already knows from
// card_price_sources/cards.
type ListingItem struct {
	Text string
	// PriceCents is only meaningful when the code was actually found with
	// a real price on the page (see MatchListingItem) - an item with no
	// price at all (out of stock, delisted, or whatever else TCG Republic
	// means by "Not Available") simply doesn't appear here, same as a
	// code that isn't on the page for any other reason. This package
	// doesn't try to distinguish those cases from each other, or surface
	// *why* a price is missing - see card_price_history's own doc comment
	// for why that's deliberately not part of the data model at all.
	PriceCents int
	// IsRanking is true for an entry that came from the "popular items"
	// sidebar widget every listing page repeats (its links carry
	// ?ref=category_page&type=ranking_product), rather than the page's
	// own paginated grid. A code appearing in both should prefer the
	// non-ranking one - see MatchListingItem.
	IsRanking bool
}

var (
	blockSplitMarker = `<li class="product_thumbnail"`
	hrefPattern      = regexp.MustCompile(`href="([^"]+)"`)
	altPattern       = regexp.MustCompile(`alt="([^"]+)"`)
	pricePattern     = regexp.MustCompile(`price_with_unit_offscreen">([0-9.]+)<`)
)

// FetchTCGRepublicListing fetches a TCG Republic category listing page
// (one of the several pages that together cover a whole set - see
// set-price-sources) and returns every product on it that has a real
// price. An item with no price (out of stock, delisted, or anything
// else) is simply omitted - see ListingItem's doc comment for why this
// package doesn't try to say more than that.
//
// Parsed with plain string-splitting + regexp rather than a real HTML
// parser (goquery/x/net/html): the page's <li class="product_thumbnail">
// structure is simple, stable, and already verified directly against live
// data, so pulling in a parsing library for this one narrow, well-
// understood shape isn't worth the new dependency. The tradeoff is real,
// though - this is more fragile to a future markup change than a real
// parser would be, since there's no structural validation, just pattern
// matching against today's known layout.
func FetchTCGRepublicListing(ctx context.Context, url string) ([]ListingItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mishis4x-collection-tracker/1.0 (personal collection price tracker)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	blocks := strings.Split(string(body), blockSplitMarker)
	var items []ListingItem
	for _, b := range blocks[1:] {
		hrefM := hrefPattern.FindStringSubmatch(b)
		altM := altPattern.FindStringSubmatch(b)
		priceM := pricePattern.FindStringSubmatch(b)
		if hrefM == nil || altM == nil || priceM == nil {
			continue
		}

		priceCents, err := parsePriceCents(priceM[1])
		if err != nil {
			continue
		}

		items = append(items, ListingItem{
			Text:       strings.ReplaceAll(altM[1], "&amp;", "&"),
			PriceCents: priceCents,
			IsRanking:  strings.Contains(hrefM[1], "ranking_product"),
		})
	}

	return items, nil
}

// parsePriceCents converts a plain decimal price string (e.g. "200.00",
// "1125.00") into integer cents. TCG Republic's listing markup always
// gives two decimal places, but this doesn't assume that - it rounds to
// the nearest cent regardless.
func parsePriceCents(s string) (int, error) {
	dollars, cents, ok := strings.Cut(s, ".")
	if !ok {
		cents = "00"
	}
	for len(cents) < 2 {
		cents += "0"
	}
	cents = cents[:2]

	wholeCents := dollars + cents
	return strconv.Atoi(wholeCents)
}

// MatchListingItem finds the item among items whose text contains code as
// a whole token - not merely as a substring - preferring a non-ranking
// (real grid) match over a ranking-widget one if both exist for the same
// code - the ranking widget can carry a stale or out-of-context price for
// a "trending" item that isn't really this page's row. Returns
// found=false if code doesn't appear at all.
//
// A plain strings.Contains was tried first and was wrong: a code like
// "BRD/W139-054S" is a literal prefix of "BRD/W139-054SSP" (that card's
// own SSP variant), so a bare substring check silently matched the wrong
// card's listing whenever both happened to appear in the same fetched
// page (e.g. the SSP variant showing up in the repeated "popular items"
// sidebar - see IsRanking) - two real cards (054S/054SSP, 029S/029SSP)
// were confirmed to have been recording each other's price this way.
// Requiring the character right after the match to not continue the code
// (not a letter or digit) is what the original Python prototype that
// generated price_sources.csv already did correctly - this brings the
// production matcher in line with that proven approach.
func MatchListingItem(items []ListingItem, code string) (item ListingItem, found bool) {
	var rankingMatch ListingItem
	var rankingFound bool
	for _, it := range items {
		if !containsCodeToken(it.Text, code) {
			continue
		}
		if !it.IsRanking {
			return it, true
		}
		rankingMatch, rankingFound = it, true
	}
	return rankingMatch, rankingFound
}

// containsCodeToken reports whether text contains code as a whole token.
// Only the trailing boundary is checked (not a leading one) - every real
// code in this domain shares the same "BRD/W139-" prefix followed by a
// fixed-shape number+letters, so a false match at the *start* of code
// isn't a realistic failure mode the way a longer suffix variant
// (S/SP/SSP/EX/...) sharing a prefix is.
func containsCodeToken(text, code string) bool {
	for start := 0; ; {
		idx := strings.Index(text[start:], code)
		if idx == -1 {
			return false
		}
		idx += start

		end := idx + len(code)
		if end >= len(text) || !isCodeContinuation(text[end]) {
			return true
		}

		start = idx + 1
	}
}

// isCodeContinuation reports whether b could be part of the same code
// token that just matched - if so, the match found so far is only a
// prefix of a longer code (e.g. "054S" inside "054SSP"), not a real
// match on its own.
func isCodeContinuation(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}
