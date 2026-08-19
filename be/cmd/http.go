package cmd

import (
	"net/http"
	"os"

	"example.com/mishis4x/handlers"
	"example.com/mishis4x/logger"
	"example.com/mishis4x/matchmaking"
	persist "example.com/mishis4x/persist"
	"github.com/gorilla/sessions"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// minSessionSecretLen is a floor, not a target - it just catches
// obviously-too-short values (e.g. "secret") before they're used to sign
// session cookies. Generate a real one with `openssl rand -hex 32`.
const minSessionSecretLen = 32

// localDevSessionSecret is the placeholder committed in
// infra/envs/local/.env.example. It's fine for local/test (that's the only
// place it's meant to be used) but must never reach a real deployment -
// loadSessionSecret refuses it for any other env, as a safety net against
// e.g. copy-pasting the local .env into a real one by mistake.
const localDevSessionSecret = "5c0d94274b26c1f78323dfd9e8de64cea15ed02ce8cdf2ae087a0f6e54d4dc50"

// loadSessionSecret reads SESSION_SECRET from the environment. It's what
// gorilla/sessions uses to sign (and, with a second key, encrypt) the
// session cookie - anyone who has it can forge a valid session for any
// user, so unlike most config this is never allowed to silently fall back
// to a default outside local/test.
func loadSessionSecret(env string) []byte {
	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		log.Fatal().Msg("SESSION_SECRET is not set - see be/infra/envs/local/.env.example")
	}
	if len(secret) < minSessionSecretLen {
		log.Fatal().Int("minLength", minSessionSecretLen).Msg("SESSION_SECRET is too short")
	}
	if env != "local" && env != "test" && secret == localDevSessionSecret {
		log.Fatal().Str("env", env).Msg("refusing to use the local/test SESSION_SECRET placeholder outside local/test")
	}
	return []byte(secret)
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
		port := 8091

		store := sessions.NewCookieStore(loadSessionSecret(env))
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
