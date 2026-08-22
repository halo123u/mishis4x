package cmd

import (
	"fmt"
	"os"
	"strconv"
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

// defaultPort is used locally and whenever PORT isn't set. Most PaaS hosts
// (Railway, Render, etc.) inject PORT and expect the app to bind to it
// rather than a fixed port, so this needs to be configurable, not just a
// local-dev nicety.
const defaultPort = 8091

func loadPort() int {
	port, err := parsePort(os.Getenv("PORT"))
	if err != nil {
		log.Fatal().Err(err).Msg("invalid PORT")
	}
	return port
}

// parsePort is split out from loadPort so it's testable without touching
// os.Exit (log.Fatal calls that directly).
func parsePort(raw string) (int, error) {
	if raw == "" {
		return defaultPort, nil
	}

	port, err := strconv.Atoi(raw)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("PORT must be a valid port number, got %q", raw)
	}

	return port, nil
}

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
		port := loadPort()

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
