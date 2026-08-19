package handlers

import (
	"context"
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

// shutdownTimeout bounds how long we wait for in-flight requests to finish
// once a shutdown signal is received before giving up and exiting anyway.
const shutdownTimeout = 10 * time.Second

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
		Addr:    fmt.Sprintf(":%d", port),
		Handler: r,
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
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

		isAuthenticated := session.Values["authenticated"]
		if isAuthenticated != nil && isAuthenticated == true {
			next.ServeHTTP(w, r)
		} else if r.URL.Path == "/api/user/login" || r.URL.Path == "/api/user/create" {
			next.ServeHTTP(w, r)
		} else {
			// TODO: add better error handling
			http.Error(w, errors.New("{}").Error(), http.StatusUnauthorized)
		}

	})
}
