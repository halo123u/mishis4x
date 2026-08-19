package cmd

import (
	"net/http"

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

		// TODO: move the signing key to a config file
		store := sessions.NewCookieStore([]byte("secret"))
		store.Options = &sessions.Options{
			Path:     "/",
			MaxAge:   86400 * 30,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			// gorilla/sessions defaults to Secure:true + SameSite=None, which
			// browsers silently refuse to store outside HTTPS - that broke
			// login entirely on local/test's plain HTTP. This app serves the
			// frontend and API from the same origin (see CLAUDE.md), so
			// SameSite=Lax is correct everywhere; only Secure needs to vary
			// by env.
			Secure: env != "local" && env != "test",
		}

		h := handlers.Data{
			P: persist.Persist{
				DB: db,
			},
			Lobby: &matchmaking.Lobby{
				Games:  []*matchmaking.Game{},
				GameID: 1,
			},
			Sessions: store,
		}
		h.InitializeHttpServer(port)

		if err := db.Close(); err != nil {
			log.Error().Err(err).Msg("error closing db connection")
		} else {
			log.Info().Msg("db connection closed")
		}
	}}
