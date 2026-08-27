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
