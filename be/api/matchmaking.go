package api

type NewGameInput struct {
	Name   string `json:"name"`
	UserId int    `json:"user_id"`
}
