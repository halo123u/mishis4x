package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"example.com/mishis4x/matchmaking"
	"example.com/mishis4x/persist"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/rs/zerolog/log"
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

	log.Fatal().Err(http.ListenAndServe(fmt.Sprintf(":%d", port), r)).Msg("http server stopped")

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
