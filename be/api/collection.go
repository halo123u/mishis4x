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
}

// AddOwnedSetInput is the POST /api/owned-sets request body - onboards
// SetID for the authenticated user.
type AddOwnedSetInput struct {
	SetID string `json:"set_id"`
}

// OwnedCardInput is one entry in SetOwnedCardsInput.Cards. CardID must
// belong to the set named by the request's {setID} path variable (checked
// server-side, not just trusted from the client) - Quantity is how many
// copies of it the user reports owning.
type OwnedCardInput struct {
	CardID   string `json:"card_id"`
	Quantity int    `json:"quantity"`
}

// SetOwnedCardsInput is the POST /api/owned-sets/{setID}/cards request
// body - the card-selection step of onboarding, submitted after
// AddOwnedSet has already onboarded the set itself.
type SetOwnedCardsInput struct {
	Cards []OwnedCardInput `json:"cards"`
}
