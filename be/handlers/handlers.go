package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"example.com/mishis4x/ebay"
	"example.com/mishis4x/matchmaking"
	"example.com/mishis4x/persist"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

const (
	// shutdownTimeout bounds how long we wait for in-flight requests to
	// finish once a shutdown signal is received before giving up and
	// exiting anyway.
	shutdownTimeout = 10 * time.Second

	// http.Server timeouts. None of these existed before, which meant a
	// slow or hung client could hold a connection open indefinitely.
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 120 * time.Second

	// dbQueryTimeout bounds how long any single DB call triggered by a
	// request is allowed to take, so a hung DB can't hang the request (and
	// the client) forever.
	dbQueryTimeout = 5 * time.Second

	// refreshPriceTimeout bounds RefreshCardPrice's whole request - it does
	// a real outbound fetch (with its own retries/backoff, see
	// pricesync.fetchListingWithRetry) rather than just a DB call, so it
	// gets a longer budget than dbQueryTimeout.
	refreshPriceTimeout = 20 * time.Second

	// maxRequestBodyBytes bounds how much of a request body we'll ever read.
	// Generous for this app's JSON API (every body is a handful of short
	// fields) - the point is capping it at all, not the exact number.
	// Without this, nothing stopped a client from sending an arbitrarily
	// large body and making the server try to read all of it.
	maxRequestBodyBytes = 1 << 20 // 1 MiB
)

// contentSecurityPolicy is scoped to what the app actually loads: same-
// origin scripts/API calls, plus Google Fonts (see fe/index.html) - not a
// generic template. Update this if the frontend starts loading from
// somewhere new, or requests will silently fail instead of erroring loudly.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' https://fonts.googleapis.com; " +
	"font-src 'self' https://fonts.gstatic.com; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"frame-ancestors 'none'"

// SessionCookieConfig holds the cookie-level settings for server-side
// sessions - everything except the actual session data, which lives in the
// DB (see persist.Session). There's no signing secret here: the cookie only
// ever carries an opaque, high-entropy random token (persist.NewSessionToken),
// and that token's own randomness is what makes it unguessable - a lookup
// against the sessions table is the actual authentication check.
type SessionCookieConfig struct {
	Name   string
	Secure bool
	TTL    time.Duration
}

type Data struct {
	P        persist.Persist
	Lobby    *matchmaking.Lobby
	Sessions SessionCookieConfig
	// LoginLimiter and SignupLimiter are deliberately separate instances -
	// see attemptLimiter's doc comment for why sharing one would let a run
	// of failed signups against an already-taken username also lock out a
	// real login for that account, and vice versa.
	LoginLimiter  *attemptLimiter
	SignupLimiter *attemptLimiter
	// InviteRequestLimiter throttles the public "request an invite" form
	// (RequestInvite) by email address - same in-memory attemptLimiter as
	// the other two, just keyed by email instead of username. Every
	// submission counts against it (there's no real "failure" case to
	// distinguish here the way a wrong password is one), so it's really
	// just a submission cap per address per window.
	InviteRequestLimiter *attemptLimiter
	// CollectionOwnerUserID and CollectionAllowAllUsers back
	// canAccessCollection/ownerOnlyMiddleware (see their doc comments) -
	// not currently wired into any route. They gated the whole collection
	// tracker to one account, because eBay's API License Agreement
	// requires eBay's express prior written consent for any "Public
	// Display" of data from their APIs, and this app has open signup. That
	// restriction never actually applied to the tracker itself though -
	// the catalog/images are TCG Republic-sourced and ownership/price-paid
	// is the user's own data, not eBay's. Kept in place (not deleted) for
	// the planned per-user market-rate/price-sync feature, which *will*
	// touch real eBay data and will need exactly this kind of gate -
	// reused there instead of guarding the general tracker.
	CollectionOwnerUserID int
	// See CollectionOwnerUserID above.
	CollectionAllowAllUsers bool
	// Ebay is nil when EBAY_APP_ID/EBAY_CERT_ID aren't configured (see
	// cmd/http.go's wiring) - GetEbayListings reports a 503 rather than
	// crashing when this is nil, so an environment that hasn't set up
	// eBay credentials yet just doesn't offer this one feature rather
	// than failing to boot. Not gated by CollectionOwnerUserID (unlike
	// what that field's own doc comment above once predicted) - see
	// GetEbayListings' doc comment for why that restriction turned out
	// not to actually apply here once eBay's real API License Agreement
	// terms were read in full.
	Ebay *ebay.Service
	// EbayListingsDisabled is a separate kill switch from Ebay being nil -
	// Ebay can be fully configured (real credentials loaded) and this
	// still forces the feature off. Added for exactly the situation that
	// prompted it: production credentials that turned out to be
	// misconfigured/rejected by eBay (a 401 invalid_client while getting
	// an oauth token), where hiding the broken feature from users while
	// it's sorted out is preferable to leaving a "Check prices" button
	// that always errors. Read by both GetGlobalData (so the frontend can
	// hide the "eBay" option entirely, not just watch it fail) and
	// GetEbayListings itself (defense in depth - the API refuses even if
	// someone bypasses the UI).
	EbayListingsDisabled bool
	// PriceTrendsEnabled gates the per-card price-trend feature (icon +
	// GET /api/sets/{setID}/price-trends) entirely - unlike
	// EbayListingsDisabled, this isn't a kill switch on something that
	// otherwise defaults on; it ships off by default (see
	// cmd.loadPriceTrendsEnabled's doc comment) since it's new and
	// hasn't been exercised against real production data yet. Read by
	// both GetGlobalData (so the frontend never shows the trend icon at
	// all while this is off) and the price-trends route itself (defense
	// in depth, same convention as EbayListingsDisabled).
	PriceTrendsEnabled bool
}

// NewData builds a Data ready to serve requests, wiring up anything with
// its own internal state (the login/signup rate limiters).
func NewData(p persist.Persist, lobby *matchmaking.Lobby, sessions SessionCookieConfig, collectionOwnerUserID int, collectionAllowAllUsers bool, ebaySvc *ebay.Service, ebayListingsDisabled bool, priceTrendsEnabled bool) *Data {
	return &Data{
		P:                       p,
		Lobby:                   lobby,
		Sessions:                sessions,
		LoginLimiter:            newAttemptLimiter(),
		SignupLimiter:           newAttemptLimiter(),
		InviteRequestLimiter:    newAttemptLimiter(),
		CollectionOwnerUserID:   collectionOwnerUserID,
		CollectionAllowAllUsers: collectionAllowAllUsers,
		Ebay:                    ebaySvc,
		EbayListingsDisabled:    ebayListingsDisabled,
		PriceTrendsEnabled:      priceTrendsEnabled,
	}
}

// NewRouter builds the app's full route table - split out from
// InitializeHttpServer so tests can exercise the real routing/middleware
// stack (via httptest.Server) instead of calling handler funcs directly and
// bypassing AuthMiddleware, security headers, etc.
func (d *Data) NewRouter() *mux.Router {
	r := mux.NewRouter()
	// Registered first so it's the outermost wrapper - catches a panic from
	// any handler *or* any other middleware below it.
	r.Use(recoverMiddleware)
	r.Use(requestLoggingMiddleware)
	r.Use(d.securityHeadersMiddleware)
	r.Use(maxBodyMiddleware)
	api := r.PathPrefix("/api").Subrouter()
	api.Use(d.AuthMiddleware)

	//API routes
	api.HandleFunc("/user/login", d.UserLogin).Methods("POST")
	api.HandleFunc("/user/create", d.UserCreate).Methods("POST")
	api.HandleFunc("/invites/request", d.RequestInvite).Methods("POST")

	// // Protected routes
	api.HandleFunc("/logout", d.UserLogout)
	api.HandleFunc("/user/password", d.ChangePassword).Methods("POST")
	api.HandleFunc("/data", d.GetGlobalData)
	api.HandleFunc("/lobbies", d.ListLobbies)
	api.HandleFunc("/lobbies/create", d.CreateLobby)

	// Collection-tracker routes: open to any authenticated user (via the
	// api subrouter's AuthMiddleware alone) - not gated by
	// ownerOnlyMiddleware. That gate existed for the eBay ToS reason
	// documented on CollectionOwnerUserID, but nothing served through
	// these routes is eBay-sourced (catalog/images come from TCG Republic,
	// ownership/price-paid is the user's own self-reported data), so there
	// was never actually a reason to restrict this to one account.
	// ownerOnlyMiddleware/canAccessCollection are deliberately left intact
	// and unused here rather than deleted - the plan is to reuse them
	// as-is for the future per-user market-rate/price-sync feature (eBay
	// data proper), gating that specifically instead of the whole tracker.
	collection := api.PathPrefix("/sets").Subrouter()
	collection.HandleFunc("", d.ListSets).Methods("GET")
	collection.HandleFunc("/{setID}/cards", d.ListCardsForSet).Methods("GET")
	collection.HandleFunc("/{setID}/price-trends", d.GetPriceTrends).Methods("GET")

	// Card images: same reasoning as above - not eBay-sourced, open to any
	// authenticated user.
	cardImages := api.PathPrefix("/cards").Subrouter()
	cardImages.HandleFunc("/{cardID}/image", d.GetCardImage).Methods("GET")
	cardImages.HandleFunc("/{cardID}/refresh-price", d.RefreshCardPrice).Methods("POST")
	cardImages.HandleFunc("/{cardID}/ebay-listings", d.GetEbayListings).Methods("GET")

	ownedSets := api.PathPrefix("/owned-sets").Subrouter()
	ownedSets.HandleFunc("", d.ListOwnedSets).Methods("GET")
	ownedSets.HandleFunc("", d.AddOwnedSet).Methods("POST")
	ownedSets.HandleFunc("/{setID}", d.DeleteOwnedSet).Methods("DELETE")
	ownedSets.HandleFunc("/{setID}/cards", d.ListOwnedCardsForSet).Methods("GET")
	ownedSets.HandleFunc("/{setID}/cards", d.SetOwnedCardsForSet).Methods("POST")

	// healthcheck
	r.PathPrefix("/healthcheck").HandlerFunc(d.Healthcheck).Methods("GET")

	r.PathPrefix(("/assets/")).Handler(http.StripPrefix("/assets/", http.FileServer(http.Dir("./dist/assets/"))))

	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve a real static file if one exists at this path (favicon,
		// robots.txt, anything else Vite copies from fe/public/ to the dist
		// root) - otherwise fall back to the SPA shell so client-side routes
		// (/login, /account, ...) still resolve. Without this, every non-
		// /assets/ request - including the favicon - silently got index.html
		// back instead of the file it actually asked for.
		requestedPath := filepath.Join("./dist", r.URL.Path)
		if info, err := os.Stat(requestedPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, requestedPath)
			return
		}
		http.ServeFile(w, r, "./dist/index.html")
	})

	return r
}

func (d *Data) InitializeHttpServer(port int) {
	log.Info().Int("port", port).Msg("starting http server")

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           d.NewRouter(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		if err != nil {
			log.Fatal().Err(err).Msg("http server failed")
		}
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("shutdown signal received, draining in-flight requests")

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("error during graceful shutdown, forcing exit")
		} else {
			log.Info().Msg("http server shut down gracefully")
		}
	}
}

// requestLoggingMiddleware logs every incoming request at debug level so it's
// available locally without drowning out info-level logs in a real deploy.
func requestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debug().Str("method", r.Method).Str("path", r.URL.Path).Msg("incoming request")
		next.ServeHTTP(w, r)
	})
}

// recoverMiddleware turns a panicking handler into a logged 500 instead of
// an unstructured stack trace printed straight to stderr by net/http's own
// (still-present) recovery - without this, a panic looks nothing like every
// other log line once something's actually watching logs on a real host.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error().
					Interface("panic", rec).
					Str("stack", string(debug.Stack())).
					Str("path", r.URL.Path).
					Msg("recovered from panic")
				writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// maxBodyMiddleware caps how much of a request body any handler will ever
// read - see maxRequestBodyBytes.
func maxBodyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// decodeJSONBody decodes r's body as JSON into v. On failure it writes the
// response itself (413 if the body exceeded maxRequestBodyBytes, 400
// otherwise) and returns false - callers should just `return` when this
// returns false, the response is already sent.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "Request body too large.")
			return false
		}
		log.Error().Err(err).Msg("error decoding request body")
		writeJSONError(w, http.StatusBadRequest, "Invalid request.")
		return false
	}
	return true
}

// securityHeadersMiddleware sets response headers that matter precisely
// because this one process serves both the HTML/JS the browser renders and
// the API it calls - there's no separate frontend server to add these to.
func (d Data) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		// HSTS only makes sense once we're actually expected to be served
		// over HTTPS - reuses the same signal already driving the session
		// cookie's Secure flag, rather than introducing a second env check.
		if d.Sessions.Secure {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// userIDContextKey is the request-context key AuthMiddleware attaches an
// authenticated request's user ID under, once it's validated the session.
type userIDContextKey struct{}

func userIDFromContext(r *http.Request) (int, bool) {
	id, ok := r.Context().Value(userIDContextKey{}).(int)
	return id, ok
}

func (d Data) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := d.sessionToken(r); token != "" {
			lookupCtx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
			session, err := d.P.GetSession(lookupCtx, token)
			cancel()

			if err == nil {
				ctx := context.WithValue(r.Context(), userIDContextKey{}, session.UserID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if !errors.Is(err, persist.ErrSessionNotFound) {
				log.Error().Err(err).Msg("error reading session")
			}
		}

		if r.URL.Path == "/api/user/login" || r.URL.Path == "/api/user/create" || r.URL.Path == "/api/invites/request" {
			next.ServeHTTP(w, r)
			return
		}

		writeJSONError(w, http.StatusUnauthorized, "You must be logged in.")
	})
}

// canAccessCollection is the rule ownerOnlyMiddleware enforces: does userID
// match CollectionOwnerUserID, or is CollectionAllowAllUsers set. Not
// currently called by any route - see CollectionOwnerUserID's doc comment
// for why this is kept rather than deleted (the planned market-rate
// feature will reuse it, gating that specifically instead of the whole
// collection tracker).
func (d Data) canAccessCollection(userID int) bool {
	return d.CollectionAllowAllUsers || (d.CollectionOwnerUserID != 0 && userID == d.CollectionOwnerUserID)
}

// ownerOnlyMiddleware restricts a route to whoever canAccessCollection
// allows. Must run after AuthMiddleware (relies on userIDFromContext
// already being set) - 401 still means "not logged in at all", this
// returns 403 for "logged in, but not allowed to see this". Not currently
// attached to any route - see canAccessCollection.
func (d Data) ownerOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromContext(r)
		if !ok || !d.canAccessCollection(userID) {
			writeJSONError(w, http.StatusForbidden, "Not available on this account.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// setSessionCookie sets the session cookie to token, using the app's
// configured TTL/Secure settings.
func (d Data) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     d.Sessions.Name,
		Value:    token,
		Path:     "/",
		MaxAge:   int(d.Sessions.TTL.Seconds()),
		HttpOnly: true,
		Secure:   d.Sessions.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie tells the browser to drop the session cookie
// immediately (MaxAge -1). Callers should also delete the corresponding row
// via d.P.DeleteSession - clearing the cookie alone doesn't revoke it.
func (d Data) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     d.Sessions.Name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   d.Sessions.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// sessionToken reads the raw session cookie value from the request, or ""
// if there isn't one.
func (d Data) sessionToken(r *http.Request) string {
	c, err := r.Cookie(d.Sessions.Name)
	if err != nil {
		return ""
	}
	return c.Value
}

// errorResponse is the JSON body returned by writeJSONError.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSONError writes a {"error": "..."} body with the given status.
// message is shown directly to the user - it must never be a raw internal
// error string (that risks leaking implementation details like SQL errors);
// log the real error separately via zerolog before calling this.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(errorResponse{Error: message}); err != nil {
		log.Error().Err(err).Msg("error writing error response")
	}
}
