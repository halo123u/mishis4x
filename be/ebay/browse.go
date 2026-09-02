package ebay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	productionTokenURL  = "https://api.ebay.com/identity/v1/oauth2/token"
	productionSearchURL = "https://api.ebay.com/buy/browse/v1/item_summary/search"
	sandboxTokenURL     = "https://api.sandbox.ebay.com/identity/v1/oauth2/token"
	sandboxSearchURL    = "https://api.sandbox.ebay.com/buy/browse/v1/item_summary/search"

	// searchLimit caps how many individual listings one query returns -
	// generous for a specific card search (few results expected, see
	// SearchQuery's doc comment), not a general-purpose search UI.
	searchLimit = 10

	requestTimeout = 10 * time.Second
)

// Listing is one individual eBay seller listing for a card - trimmed down
// to just what mishis4x's UI actually shows, not the ~50 other fields the
// real ItemSummary response includes (return policy, tax, aspects, etc.).
type Listing struct {
	ItemID                   string
	Title                    string
	PriceCents               int
	Condition                string
	SellerUsername           string
	SellerFeedbackPercentage string
	ItemWebURL               string
	ImageURL                 string
}

// Config configures a Service - AppID/CertID are the app-level OAuth2
// Client Credentials Grant secrets (see [[ebay-api-license-terms]] for
// why no per-user credentials are needed for this). Sandbox picks between
// eBay's sandbox (fake fixture data, safe to hit freely) and production
// (real listings) hosts.
type Config struct {
	AppID   string
	CertID  string
	Sandbox bool
}

// Service is the app's one eBay Browse API client - an OAuth2 token
// manager plus a bounded, TTL'd cache of each card's most recently fetched
// listings (see listingsCache's doc comment for why this is a cache, not
// a database table: eBay's own API License Agreement permits caching
// item listing data up to 6h, and this mirrors that directly rather than
// inventing a different staleness rule).
type Service struct {
	tokens    *tokenManager
	cache     *listingsCache
	client    *http.Client
	searchURL string
}

// NewService builds a Service against eBay's real sandbox/production
// hosts, per cfg.Sandbox.
func NewService(cfg Config) *Service {
	tokenURL, searchURL := productionTokenURL, productionSearchURL
	if cfg.Sandbox {
		tokenURL, searchURL = sandboxTokenURL, sandboxSearchURL
	}
	return NewServiceWithURLs(cfg.AppID, cfg.CertID, tokenURL, searchURL)
}

// NewServiceWithURLs is the same construction NewService does, but with
// explicit token/search URLs - exported for tests (in this package and
// be/handlers) to point at a local httptest.Server instead of eBay's real
// hosts, same convention pricesync's tests use for TCG Republic. Not
// meant for production use - call NewService there instead.
func NewServiceWithURLs(appID, certID, tokenURL, searchURL string) *Service {
	client := &http.Client{Timeout: requestTimeout}
	return &Service{
		tokens:    newTokenManager(appID, certID, tokenURL, client),
		cache:     newListingsCache(cacheCapacity, cacheTTL),
		client:    client,
		searchURL: searchURL,
	}
}

// SearchQuery builds the same search string
// fe/src/ebay.ts's ebaySearchUrl already uses for its plain search-link
// mode - eBay listing titles key off a card's short number (e.g. "086S"),
// not its full catalog code with set prefix ("BRD/W139-086S"), so
// searching with the full code returns few or no results. Kept as its own
// function (not inlined into the handler) so it stays a direct textual
// twin of the frontend's version rather than silently drifting apart.
func SearchQuery(setName, code string) string {
	parts := strings.Split(code, "-")
	shortCode := parts[len(parts)-1]
	if setName == "" {
		return shortCode
	}
	return setName + " " + shortCode
}

// GetListings returns cardID's current eBay listings for query, serving a
// cached result if one exists and isn't past cacheTTL, otherwise fetching
// live from eBay and caching the result before returning it.
func (s *Service) GetListings(ctx context.Context, cardID, query string) ([]Listing, error) {
	if listings, ok := s.cache.get(cardID); ok {
		return listings, nil
	}

	listings, err := s.searchLive(ctx, query)
	if err != nil {
		return nil, err
	}

	s.cache.set(cardID, listings)
	return listings, nil
}

type searchResponse struct {
	ItemSummaries []struct {
		ItemID string `json:"itemId"`
		Title  string `json:"title"`
		Price  struct {
			Value string `json:"value"`
		} `json:"price"`
		Condition string `json:"condition"`
		Seller    struct {
			Username           string `json:"username"`
			FeedbackPercentage string `json:"feedbackPercentage"`
		} `json:"seller"`
		ItemWebURL string `json:"itemWebUrl"`
		Image      struct {
			ImageURL string `json:"imageUrl"`
		} `json:"image"`
	} `json:"itemSummaries"`
}

// searchLive calls the Browse API's item_summary/search live - sort=price
// so the cheapest listing comes first, matching the same "lowest first"
// default fe/src/ebay.ts's plain search link already uses (_sop=15).
func (s *Service) searchLive(ctx context.Context, query string) ([]Listing, error) {
	token, err := s.tokens.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting oauth token: %w", err)
	}

	reqURL := fmt.Sprintf("%s?q=%s&limit=%d&sort=price", s.searchURL, url.QueryEscape(query), searchLimit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building search request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	// Required by the Browse API, not optional - see the request-headers
	// reference table's Marketplace ID values. US-only for now, same scope
	// as the rest of this app's eBay integration.
	req.Header.Set("X-EBAY-C-MARKETPLACE-ID", "EBAY_US")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", query, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d searching %q", resp.StatusCode, query)
	}

	var parsed searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding search response: %w", err)
	}

	listings := make([]Listing, 0, len(parsed.ItemSummaries))
	for _, item := range parsed.ItemSummaries {
		cents, err := parsePriceCents(item.Price.Value)
		if err != nil {
			// A listing with an unparseable price is skipped rather than
			// failing the whole search - same tolerance a malformed CSV
			// row or a not-found TCG Republic card gets elsewhere in this
			// codebase.
			continue
		}
		listings = append(listings, Listing{
			ItemID:                   item.ItemID,
			Title:                    item.Title,
			PriceCents:               cents,
			Condition:                item.Condition,
			SellerUsername:           item.Seller.Username,
			SellerFeedbackPercentage: item.Seller.FeedbackPercentage,
			ItemWebURL:               item.ItemWebURL,
			ImageURL:                 item.Image.ImageURL,
		})
	}

	return listings, nil
}

// parsePriceCents converts eBay's decimal-string price ("42.00") into
// integer cents, the same representation the rest of this app's price
// data (card_price_history.price_cents) already uses. Deliberately its
// own small implementation rather than reusing pricesync's - same idea,
// different source format, and not worth a shared package for one
// ~20-line helper used by exactly two callers.
func parsePriceCents(value string) (int, error) {
	parts := strings.SplitN(value, ".", 2)
	dollars, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("parsing dollars from %q: %w", value, err)
	}

	cents := 0
	if len(parts) == 2 {
		centsStr := parts[1]
		if len(centsStr) == 1 {
			centsStr += "0" // "5.5" -> 550 cents, not 55
		}
		if len(centsStr) > 2 {
			centsStr = centsStr[:2]
		}
		cents, err = strconv.Atoi(centsStr)
		if err != nil {
			return 0, fmt.Errorf("parsing cents from %q: %w", value, err)
		}
	}

	return dollars*100 + cents, nil
}
