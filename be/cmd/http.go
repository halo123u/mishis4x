package cmd

import (
	"example.com/mishis4x/handlers"
	"example.com/mishis4x/logger"
	"example.com/mishis4x/matchmaking"
	persist "example.com/mishis4x/persist"
	"github.com/gorilla/sessions"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func init() {
	httpCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to run migrations on")
	rootCMD.AddCommand(httpCMD)
}

var httpCMD = &cobra.Command{
	Use:   "http",
	Short: "Start the HTTP server",
	Long:  `Start the HTTP server`,
	Run: func(cmd *cobra.Command, args []string) {
		logger.Init(env)

		db, err := persist.NewDB(env)
		if err != nil {
			log.Fatal().Err(err).Msg("error connecting to db")
		}

		db.SetMaxOpenConns(5)
		port := 8091

		h := handlers.Data{
			P: persist.Persist{
				DB: db,
			},
			Lobby: &matchmaking.Lobby{
				Games:  []*matchmaking.Game{},
				GameID: 1,
			},
			// TODO: move this to a config file
			Sessions: sessions.NewCookieStore([]byte("secret")),
		}
		h.InitializeHttpServer(port)

	}}
