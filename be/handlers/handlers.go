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
	"github.com/gorilla/sessions"
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

type Data struct {
	P        persist.Persist
	Lobby    *matchmaking.Lobby
	Sessions *sessions.CookieStore
}

func (d *Data) InitializeHttpServer(port int) {
	log.Info().Int("port", port).Msg("starting http server")
	r := mux.NewRouter()
	r.Use(requestLoggingMiddleware)
	api := r.PathPrefix("/api").Subrouter()
	api.Use(d.AuthMiddleware)

	//API routes
	api.HandleFunc("/user/login", d.UserLogin).Methods("POST")
	api.HandleFunc("/user/create", d.UserCreate).Methods("POST")

	// // Protected routes
	api.HandleFunc("/logout", d.UserLogout)
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

func (d Data) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := d.Sessions.Get(r, "session")
		if err != nil {
			log.Error().Err(err).Str("path", r.URL.Path).Msg("error reading session")
			writeJSONError(w, http.StatusBadRequest, "Invalid session.")
			return
		}

		isAuthenticated, _ := session.Values["authenticated"].(bool)
		if isAuthenticated {
			next.ServeHTTP(w, r)
			return
		}

		if r.URL.Path == "/api/user/login" || r.URL.Path == "/api/user/create" {
			next.ServeHTTP(w, r)
			return
		}

		writeJSONError(w, http.StatusUnauthorized, "You must be logged in.")
	})
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
