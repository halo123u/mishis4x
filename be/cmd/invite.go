package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"example.com/mishis4x/email"
	"example.com/mishis4x/logger"
	"example.com/mishis4x/persist"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// defaultInviteFromAddress is used when EMAIL_FROM_ADDRESS isn't set -
// mishis4x.com is verified with Resend specifically for this purpose (see
// invite-approve's own doc comment).
const defaultInviteFromAddress = "invites@mishis4x.com"

func init() {
	rootCMD.AddCommand(inviteListCMD)
	inviteListCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to connect to")

	rootCMD.AddCommand(inviteApproveCMD)
	inviteApproveCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to connect to")

	rootCMD.AddCommand(inviteDenyCMD)
	inviteDenyCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to connect to")
}

var inviteListCMD = &cobra.Command{
	Use:   "invite-list",
	Short: "List invite requests still awaiting approval",
	Long: `Signup is invite-only (see handlers.UserCreate) - there's no
self-service minting. Every invite starts with someone submitting the
public "request an invite" form, which lands here with status
'requested'. This lists exactly those, oldest first, so you know which
id to pass to invite-approve/invite-deny.`,
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

		requests, err := p.ListRequestedInvites(context.Background())
		if err != nil {
			log.Fatal().Err(err).Msg("error listing invite requests")
		}

		if len(requests) == 0 {
			fmt.Println("no pending invite requests")
			return
		}

		for _, r := range requests {
			fmt.Printf("%d\t%s\n", r.ID, r.EmailAddress)
		}
	},
}

var inviteApproveCMD = &cobra.Command{
	Use:   "invite-approve <id>",
	Short: "Approve a pending invite request and email its code",
	Long: `Flips a 'requested' invite (see invite-list for the id) to
'approved' and sends the code out over email via Resend
(https://resend.com) - approving is the only thing that actually reveals
a code to anyone.

Requires RESEND_API_KEY (from your Resend account) and APP_BASE_URL (the
public URL to build the sign-up link against, e.g.
https://mishis4x.com) to be set. EMAIL_FROM_ADDRESS defaults to
invites@mishis4x.com if unset - either way, the sending address's domain
must already be verified with Resend (SPF/DKIM DNS records) or the send
itself will fail even though the DB update already succeeded.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logger.Init(env)

		id, err := strconv.Atoi(args[0])
		if err != nil {
			log.Fatal().Str("id", args[0]).Msg("invite id must be a number - see invite-list")
		}

		// Both read/validated before ever touching the DB - once
		// ApproveInvite below succeeds, the request is no longer
		// 'requested' and can't be re-approved, so a config problem
		// discovered only after that point would strand an approved code
		// nobody can see. Fail here instead, while it's still cheap to
		// just fix the env and re-run.
		appBaseURL := loadAppBaseURL()
		if appBaseURL == "" {
			log.Fatal().Msg("APP_BASE_URL is not set")
		}
		emailSvc, err := loadEmailService()
		if err != nil {
			log.Fatal().Err(err).Msg("error configuring email")
		}

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

		req, err := p.ApproveInvite(context.Background(), id)
		if err != nil {
			log.Fatal().Err(err).Int("id", id).Msg("error approving invite")
		}

		signupURL := fmt.Sprintf("%s/sign-up?invite=%s", appBaseURL, req.Code)
		if err := emailSvc.SendInviteEmail(context.Background(), req.EmailAddress, signupURL); err != nil {
			// Already approved in the DB at this point - not rolled back
			// (re-running invite-approve on this id would just fail, it's
			// no longer 'requested'). Print the link so it isn't stranded
			// unsendable; share it manually.
			log.Error().Err(err).Str("email", req.EmailAddress).Msg("approved, but sending the invite email failed - link printed below")
			fmt.Printf("approved - email send failed, share this link manually: %s\n", signupURL)
			return
		}

		fmt.Printf("approved and emailed %s\n", req.EmailAddress)
	},
}

var inviteDenyCMD = &cobra.Command{
	Use:   "invite-deny <id>",
	Short: "Deny a pending invite request",
	Long: `Flips a 'requested' invite (see invite-list for the id) to
'denied' - no email is ever sent, and the code never leaves the DB.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logger.Init(env)

		id, err := strconv.Atoi(args[0])
		if err != nil {
			log.Fatal().Str("id", args[0]).Msg("invite id must be a number - see invite-list")
		}

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

		req, err := p.DenyInvite(context.Background(), id)
		if err != nil {
			log.Fatal().Err(err).Int("id", id).Msg("error denying invite")
		}

		fmt.Printf("denied request from %s\n", req.EmailAddress)
	},
}

// loadAppBaseURL reads APP_BASE_URL - the public URL to build a sign-up
// link against (e.g. https://mishis4x.com) - trimmed of whitespace (see
// invite-approve's own doc comment for the real, hit-in-production
// mistake this guards against: a trailing space silently corrupting
// every generated link without ever failing an == "" check). Returns ""
// if unset; callers decide how severely to treat that (invite-approve
// treats it as fatal, the http server's admin routes just disable
// approving via that path).
func loadAppBaseURL() string {
	return strings.TrimSpace(os.Getenv("APP_BASE_URL"))
}

// loadEmailService reads RESEND_API_KEY (required) and
// EMAIL_FROM_ADDRESS (defaults to defaultInviteFromAddress). Returns an
// error rather than fataling directly - invite-approve calls this before
// touching the DB specifically so a missing/bad config is caught before
// an invite gets irreversibly flipped to 'approved'.
//
// TrimSpace on both env vars for the same reason buildSignupURL's
// APP_BASE_URL read has it (see invite-approve) - a stray copy-pasted
// space wouldn't fail the == "" check, it'd just quietly break the
// value at the point it's actually used (a malformed Authorization
// header, an invalid From address).
func loadEmailService() (*email.Service, error) {
	apiKey := strings.TrimSpace(os.Getenv("RESEND_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("RESEND_API_KEY is not set")
	}

	from := strings.TrimSpace(os.Getenv("EMAIL_FROM_ADDRESS"))
	if from == "" {
		from = defaultInviteFromAddress
	}

	return email.NewService(apiKey, from), nil
}
