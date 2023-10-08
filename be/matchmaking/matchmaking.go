package matchmaking

import "example.com/mishis4x/api"

type Game struct {
	Id          int
	Name        string
	CreatedById int
	PlayerIds   []int
	Winner      int
	Status      string
}

// Todo: improve game id uniqueness
type Lobby struct {
	Games  []*Game
	GameID int
}

func (l *Lobby) ListGames() []*Game {
	return l.Games
}

// Todo: will move this to get it from a token/cookie? basically make it safer

func (l *Lobby) AddGame(i *api.NewGameInput) error {

	newGame := &Game{
		Status:      "Active",
		Winner:      -1,
		PlayerIds:   []int{},
		Name:        i.Name,
		CreatedById: i.UserId,
	}

	l.Games = append(l.Games, newGame)
	l.GameID++

	return nil
}
