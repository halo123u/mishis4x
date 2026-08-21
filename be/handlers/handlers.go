package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	P            persist.Persist
	Lobby        *matchmaking.Lobby
	Sessions     SessionCookieConfig
	LoginLimiter *loginLimiter
}

// NewData builds a Data ready to serve requests, wiring up anything with
// its own internal state (currently just the login rate limiter).
func NewData(p persist.Persist, lobby *matchmaking.Lobby, sessions SessionCookieConfig) *Data {
	return &Data{
		P:            p,
		Lobby:        lobby,
		Sessions:     sessions,
		LoginLimiter: newLoginLimiter(),
	}
}

func (d *Data) InitializeHttpServer(port int) {
	log.Info().Int("port", port).Msg("starting http server")
	r := mux.NewRouter()
	r.Use(requestLoggingMiddleware)
	r.Use(d.securityHeadersMiddleware)
	api := r.PathPrefix("/api").Subrouter()
	api.Use(d.AuthMiddleware)

	//API routes
	api.HandleFunc("/user/login", d.UserLogin).Methods("POST")
	api.HandleFunc("/user/create", d.UserCreate).Methods("POST")

	// // Protected routes
	api.HandleFunc("/logout", d.UserLogout)
	api.HandleFunc("/user/password", d.ChangePassword).Methods("POST")
	api.HandleFunc("/data", d.GetGlobalData)
	api.HandleFunc("/lobbies", d.ListLobbies)
	api.HandleFunc("/lobbies/create", d.CreateLobby)

	// healthcheck
	r.PathPrefix("/healthcheck").HandlerFunc(d.Healthcheck).Methods("GET")

	r.PathPrefix(("/assets/")).Handler(http.StripPrefix("/assets/", http.FileServer(http.Dir("./dist/assets/"))))

	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./dist/index.html")
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           r,
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

		if r.URL.Path == "/api/user/login" || r.URL.Path == "/api/user/create" {
			next.ServeHTTP(w, r)
			return
		}

		writeJSONError(w, http.StatusUnauthorized, "You must be logged in.")
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
