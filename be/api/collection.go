package api

import "time"

type Set struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	CardCount   int        `json:"card_count"`
	ReleaseDate *time.Time `json:"release_date,omitempty"`
	Status      string     `json:"status"`
}

type Card struct {
	ID     string `json:"id"`
	SetID  string `json:"set_id"`
	Name   string `json:"name"`
	Code   string `json:"code"`
	Rarity string `json:"rarity"`
	// MarketPriceCents is the most recently scraped price for this card
	// (see card_price_history), in cents like PricePaidCents. Omitted
	// entirely - not zero - when there's no current price to show, same
	// nil-means-unknown convention as PricePaidCents.
	MarketPriceCents *int `json:"market_price_cents,omitempty"`
	// MarketCheckedAt is when this card's price was last actually checked,
	// regardless of whether that check found a price - present with
	// MarketPriceCents absent means "checked, nothing available" (e.g. out
	// of stock); absent entirely means this card has never been checked at
	// all (no price source configured, or not synced yet).
	MarketCheckedAt *time.Time `json:"market_checked_at,omitempty"`
	// MarketURL is where this card's price was checked - card_price_sources'
	// own scrape-source url (a TCG Republic category listing page, not a
	// page dedicated to this one card - see set-price-sources's doc
	// comment). Good enough for a "see this on TCG Republic" link even
	// though it isn't card-specific. Omitted when there's no source
	// configured for this card at all, same condition as the other
	// market_* fields.
	MarketURL string `json:"market_url,omitempty"`
}

// AddOwnedSetInput is the POST /api/owned-sets request body - onboards
// SetID for the authenticated user.
type AddOwnedSetInput struct {
	SetID string `json:"set_id"`
}

// OwnedCardCopyInput is one physical copy of a card (#108) - the unit
// OwnedCardInput.Copies is a list of. ID is empty for a copy that doesn't
// exist yet (the client is asking to create one); non-empty to reference
// (and update the price of) a copy the server already has. Server-scoped
// by user_id/card_id on write - an ID that doesn't actually belong to the
// authenticated user's copy of this card is silently ignored rather than
// erroring (see persist.reconcileOwnedCardCopiesByID), the same
// fail-safe-not-fail-loud posture as a garbage card_id being rejected
// earlier in the request instead of ever reaching this far.
type OwnedCardCopyInput struct {
	ID             string `json:"id,omitempty"`
	PricePaidCents *int   `json:"price_paid_cents,omitempty"`
}

// OwnedCardInput is one entry in SetOwnedCardsInput.Cards, and also what
// GET /api/owned-sets/{setID}/cards returns a list of - the same shape
// serves as both the write payload and the read response, since there's
// nothing input-specific about it (no server-generated fields to omit on
// the way out). CardID must belong to the set named by the request's
// {setID} path variable (checked server-side, not just trusted from the
// client).
//
// Quantity/PricePaidCents are the read-side aggregate - a plain
// len()/sum() over Copies - kept for callers that only need the total
// (SetDetail, DeckInsights) so they don't need to change for #108 at all.
// They're ignored on write: Copies is authoritative there. PricePaidCents
// is in cents, not a decimal dollar amount (avoids float rounding on money
// entirely); nil/omitted means unknown, not $0.
//
// Copies is the real per-copy detail (#108): on read, every copy the user
// owns of this card, oldest first, each with its own id and price. On
// write, it's the client's full desired end state for this card - any
// existing copy whose id isn't present gets deleted, each id that is
// present gets its price updated to match, and each copy with no id gets
// created fresh (see persist.CardQuantity.Copies).
type OwnedCardInput struct {
	CardID         string               `json:"card_id"`
	Quantity       int                  `json:"quantity"`
	PricePaidCents *int                 `json:"price_paid_cents,omitempty"`
	Copies         []OwnedCardCopyInput `json:"copies,omitempty"`
}

// SetOwnedCardsInput is the POST /api/owned-sets/{setID}/cards request
// body - the card-selection step of onboarding, submitted after
// AddOwnedSet has already onboarded the set itself.
type SetOwnedCardsInput struct {
	Cards []OwnedCardInput `json:"cards"`
}
