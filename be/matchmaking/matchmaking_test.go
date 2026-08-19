package matchmaking

import (
	"testing"

	"example.com/mishis4x/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddGame(t *testing.T) {
	l := &Lobby{Games: []*Game{}, GameID: 1}

	err := l.AddGame(&api.NewGameInput{Name: "first game", UserId: 42})
	require.NoError(t, err)

	require.Len(t, l.Games, 1)
	game := l.Games[0]
	assert.Equal(t, "first game", game.Name)
	assert.Equal(t, 42, game.CreatedById)
	assert.Equal(t, "Active", game.Status)
	assert.Equal(t, -1, game.Winner)
	assert.Equal(t, []int{}, game.PlayerIds)
}

func TestAddGame_IncrementsGameID(t *testing.T) {
	l := &Lobby{Games: []*Game{}, GameID: 1}

	require.NoError(t, l.AddGame(&api.NewGameInput{Name: "a", UserId: 1}))
	require.NoError(t, l.AddGame(&api.NewGameInput{Name: "b", UserId: 2}))

	assert.Equal(t, 3, l.GameID)
	require.Len(t, l.Games, 2)
	assert.Equal(t, "a", l.Games[0].Name)
	assert.Equal(t, "b", l.Games[1].Name)
}

func TestListGames(t *testing.T) {
	l := &Lobby{Games: []*Game{}, GameID: 1}
	assert.Empty(t, l.ListGames())

	require.NoError(t, l.AddGame(&api.NewGameInput{Name: "only game", UserId: 7}))

	games := l.ListGames()
	require.Len(t, games, 1)
	assert.Equal(t, "only game", games[0].Name)
}
