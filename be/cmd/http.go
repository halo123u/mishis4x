package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"example.com/mishis4x/ebay"
	"example.com/mishis4x/handlers"
	"example.com/mishis4x/logger"
	"example.com/mishis4x/matchmaking"
	persist "example.com/mishis4x/persist"
	"example.com/mishis4x/pricesync"
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

// priceSyncInterval is how often the background job re-checks every
// configured price source (see pricesync.SyncAll) once enabled. Coarse on
// purpose - TCG Republic prices don't move fast enough to need more than
// this, and running it in-process (rather than a separate scheduled
// process/cron) keeps the whole app self-contained, matching the "one
// binary" deployment philosophy - no external scheduler to stand up.
const priceSyncInterval = 12 * time.Hour

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

// loadEbayService reads EBAY_APP_ID/EBAY_CERT_ID (and EBAY_SANDBOX) and
// builds an ebay.Service, or returns nil if either credential is unset -
// handlers.Data.GetEbayListings reports a 503 rather than crashing when
// this is nil, so an environment that hasn't configured eBay credentials
// yet just doesn't offer that one feature rather than failing to boot.
// EBAY_SANDBOX defaults to true (unset = sandbox) - the safer default
// given a misconfigured/forgotten env var should mean "fake fixture
// data," not "accidentally querying eBay's real production API."
func loadEbayService() *ebay.Service {
	appID := os.Getenv("EBAY_APP_ID")
	certID := os.Getenv("EBAY_CERT_ID")
	if appID == "" || certID == "" {
		log.Warn().Msg("EBAY_APP_ID/EBAY_CERT_ID not set, eBay listings endpoint disabled")
		return nil
	}

	sandbox := true
	if raw := os.Getenv("EBAY_SANDBOX"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			log.Fatal().Err(err).Str("EBAY_SANDBOX", raw).Msg("invalid EBAY_SANDBOX")
		}
		sandbox = parsed
	}

	log.Info().Bool("sandbox", sandbox).Msg("eBay listings endpoint enabled")
	return ebay.NewService(ebay.Config{AppID: appID, CertID: certID, Sandbox: sandbox})
}

// loadEbayListingsDisabled reads EBAY_LISTINGS_DISABLED - a separate kill
// switch from loadEbayService's nil-Service case (see
// handlers.Data.EbayListingsDisabled's doc comment for why: credentials
// can be fully configured and still need to be hidden, e.g. while a
// production auth failure gets sorted out). Unset/empty returns false -
// the feature stays on by default, same as before this switch existed;
// this is an opt-in "turn it off," not a fail-closed gate.
func loadEbayListingsDisabled() bool {
	raw := os.Getenv("EBAY_LISTINGS_DISABLED")
	if raw == "" {
		return false
	}

	disabled, err := strconv.ParseBool(raw)
	if err != nil {
		log.Fatal().Err(err).Str("EBAY_LISTINGS_DISABLED", raw).Msg("invalid EBAY_LISTINGS_DISABLED")
	}
	if disabled {
		log.Warn().Msg("eBay listings feature disabled via EBAY_LISTINGS_DISABLED")
	}

	return disabled
}

// loadEnablePriceSync reads ENABLE_PRICE_SYNC - an explicit opt-in for the
// background price-sync loop, same fail-closed-by-default convention as
// loadCollectionAllowAllUsers. Unset/empty returns false: without this,
// every local `be http` restart or CI's docker-compose e2e run (both run
// against real infra, not a mock) would otherwise make real, repeated
// requests against TCG Republic - not something that should happen just
// because a dev restarted the server or a PR triggered CI.
func loadEnablePriceSync() bool {
	raw := os.Getenv("ENABLE_PRICE_SYNC")
	if raw == "" {
		return false
	}

	enable, err := strconv.ParseBool(raw)
	if err != nil {
		log.Fatal().Err(err).Str("ENABLE_PRICE_SYNC", raw).Msg("invalid ENABLE_PRICE_SYNC")
	}

	return enable
}

// runPriceSyncLoop runs pricesync.SyncAll on interval until ctx is
// canceled (interval is always priceSyncInterval outside tests - taken as
// a parameter so tests can use a short one instead of actually waiting).
// Deliberately does NOT run an initial pass immediately at startup:
// SyncAll re-checks every configured source unconditionally (no per-card
// staleness filter yet), so an immediate run on every boot would mean a
// crash-loop (the server repeatedly restarting for some unrelated reason)
// hammers TCG Republic with a real full sync each time - waiting for the
// first real tick is the safer default until staleness-aware partial
// syncing exists.
func runPriceSyncLoop(ctx context.Context, p *persist.Persist, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runPriceSyncTick(ctx, p)
		}
	}
}

// runPriceSyncTick runs one SyncAll pass with the same panic-recovery
// discipline recoverMiddleware gives every HTTP handler - a goroutine
// panic that reaches the top of its own stack unrecovered takes down the
// *entire process*, not just this goroutine, so this needs its own
// recover() rather than relying on anything HTTP-request-shaped. Between
// this and SyncAll's own returned-error handling below, nothing this loop
// can hit should ever be able to crash be http - worst case for now is a
// logged failure and next tick trying again, on the expectation that
// someone checks the logs; a real "last successful sync" surfaced in the
// UI (from the market_checked_at data already returned by the API) is
// planned as a later, separate step rather than done here.
func runPriceSyncTick(ctx context.Context, p *persist.Persist) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Error().
				Interface("panic", rec).
				Str("stack", string(debug.Stack())).
				Msg("recovered from panic in background price sync")
		}
	}()

	if _, err := pricesync.SyncAll(ctx, p); err != nil {
		log.Error().Err(err).Msg("background price sync failed")
	}
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
			loadEbayService(),
			loadEbayListingsDisabled(),
		)

		// Shares the same DB pool/connection the request handlers already
		// use, rather than opening a second one just for this - canceled
		// right after InitializeHttpServer returns (below), before db is
		// closed, so a sync in progress gets a chance to notice ctx.Done()
		// between urls rather than racing a closed connection.
		syncCtx, cancelSync := context.WithCancel(context.Background())
		var syncWG sync.WaitGroup
		if loadEnablePriceSync() {
			syncWG.Add(1)
			go func() {
				defer syncWG.Done()
				runPriceSyncLoop(syncCtx, &persist.Persist{DB: db}, priceSyncInterval)
			}()
		}

		h.InitializeHttpServer(port)

		cancelSync()
		syncWG.Wait()

		if err := db.Close(); err != nil {
			log.Error().Err(err).Msg("error closing db connection")
		} else {
			log.Info().Msg("db connection closed")
		}
	}}
