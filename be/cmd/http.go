package cmd

import (
	"time"

	"example.com/mishis4x/handlers"
	"example.com/mishis4x/logger"
	"example.com/mishis4x/matchmaking"
	persist "example.com/mishis4x/persist"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// sessionTTL is how long a session stays valid (and how long the browser
// keeps the cookie) after login/signup.
const sessionTTL = 30 * 24 * time.Hour

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

		h := handlers.NewData(
			persist.Persist{DB: db},
			&matchmaking.Lobby{
				Games:  []*matchmaking.Game{},
				GameID: 1,
			},
			handlers.SessionCookieConfig{
				Name: "session",
				TTL:  sessionTTL,
				// Sessions are opaque random tokens looked up server-side
				// (see persist/sessions.go) - a browser will silently refuse
				// to store a Secure cookie outside HTTPS, so this only turns
				// on once we're not on local/test's plain HTTP.
				Secure: env != "local" && env != "test",
			},
		)
		h.InitializeHttpServer(port)

		if err := db.Close(); err != nil {
			log.Error().Err(err).Msg("error closing db connection")
		} else {
			log.Info().Msg("db connection closed")
		}
	}}
