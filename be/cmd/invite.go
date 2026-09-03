package cmd

import (
	"context"
	"fmt"

	"example.com/mishis4x/logger"
	"example.com/mishis4x/persist"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func init() {
	rootCMD.AddCommand(inviteCreateCMD)
	inviteCreateCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to connect to")
}

var inviteCreateCMD = &cobra.Command{
	Use:   "invite-create",
	Short: "Mint a one-time invite token, required for signup",
	Long: `Signup is invite-only, not open public registration (see
handlers.UserCreate) - this mints a single fresh token in the invites
table and prints it, ready to append to the sign-up page as
?invite=<token>.

The token is single-use: it's atomically claimed the moment someone
submits the sign-up form with it (see persist.RedeemInvite), whether or
not that signup actually succeeds - a failed signup (e.g. a duplicate
username) burns the invite rather than leaving it reusable, so mint a
new one if that happens. The one exception is a request rejected by the
per-username signup rate limiter (see handlers.UserCreate) - that's
checked before the invite is claimed, so a lockout doesn't burn one.

There's no separate "list" or "revoke" command yet - this is deliberately
the one operation actually needed today, not a speculative full CRUD
surface. Add more as they're actually needed.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger.Init(env)

		db, err := persist.NewDB(env)
		if err != nil {
			log.Fatal().Err(err).Msg("error connecting to db")
		}
		defer func() {
			if closeErr := db.Close(); closeErr != nil {
				log.Error().Err(closeErr).Msg("error closing db connection")
			}
		}()
		p := &persist.Persist{DB: db}

		token, err := p.CreateInvite(context.Background())
		if err != nil {
			log.Fatal().Err(err).Msg("error creating invite")
		}

		// Deliberately no base URL prepended - there's no configured
		// public hostname to draw one from yet (see the "get a domain"
		// task this was filed alongside), and guessing one (localhost?
		// the DigitalOcean-assigned hostname?) would just be wrong most
		// of the time. Print the path, let the caller prepend whatever
		// their actual current URL is.
		fmt.Printf("invite created: /sign-up?invite=%s\n", token)
	},
}
