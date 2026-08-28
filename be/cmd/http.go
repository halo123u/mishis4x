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

// DB connection pool settings. None of these existed before beyond
// SetMaxOpenConns - matters more the moment this is on a managed DB that
// recycles idle connections server-side rather than a throwaway local one.
const (
	dbMaxOpenConns    = 5
	dbMaxIdleConns    = 5
	dbConnMaxLifetime = 5 * time.Minute
	dbConnMaxIdleTime = 2 * time.Minute
)

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

// loadCollectionOwnerUserID reads COLLECTION_OWNER_USER_ID - the one users.id
// allowed to see collection-tracker routes (see
// handlers.Data.CollectionOwnerUserID's doc comment for why). Unset/empty
// returns 0, which handlers.ownerOnlyMiddleware treats as "nobody" rather
// than "everybody" - failing closed by default, not open.
func loadCollectionOwnerUserID() int {
	raw := os.Getenv("COLLECTION_OWNER_USER_ID")
	if raw == "" {
		return 0
	}

	id, err := strconv.Atoi(raw)
	if err != nil {
		log.Fatal().Err(err).Str("COLLECTION_OWNER_USER_ID", raw).Msg("invalid COLLECTION_OWNER_USER_ID")
	}

	return id
}

// loadCollectionAllowAllUsers reads COLLECTION_ALLOW_ALL_USERS - an
// explicit opt-out of the COLLECTION_OWNER_USER_ID restriction (see
// handlers.Data.CollectionAllowAllUsers's doc comment for why this exists).
// Unset/empty returns false, so any environment that doesn't set this stays
// on the original fail-closed behavior by default.
func loadCollectionAllowAllUsers() bool {
	raw := os.Getenv("COLLECTION_ALLOW_ALL_USERS")
	if raw == "" {
		return false
	}

	allow, err := strconv.ParseBool(raw)
	if err != nil {
		log.Fatal().Err(err).Str("COLLECTION_ALLOW_ALL_USERS", raw).Msg("invalid COLLECTION_ALLOW_ALL_USERS")
	}

	return allow
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

		db.SetMaxOpenConns(dbMaxOpenConns)
		db.SetMaxIdleConns(dbMaxIdleConns)
		db.SetConnMaxLifetime(dbConnMaxLifetime)
		db.SetConnMaxIdleTime(dbConnMaxIdleTime)

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
			loadCollectionOwnerUserID(),
			loadCollectionAllowAllUsers(),
		)
		h.InitializeHttpServer(port)

		if err := db.Close(); err != nil {
			log.Error().Err(err).Msg("error closing db connection")
		} else {
			log.Info().Msg("db connection closed")
		}
	}}
