package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"example.com/mishis4x/matchmaking"
	"example.com/mishis4x/persist"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

type Data struct {
	P        persist.Persist
	Lobby    *matchmaking.Lobby
	Sessions *sessions.CookieStore
}

func (d *Data) InitializeHttpServer(port int) {

	r := mux.NewRouter()
	s := r.PathPrefix("/").Subrouter()
	s.Use(d.AuthMiddleware)

	r.HandleFunc("/user/login", d.UserLogin)
	r.HandleFunc("/user/create", d.UserCreate)

	// Protected routes
	s.HandleFunc("/user/data", d.GetUserData)
	s.HandleFunc("/lobbies", d.ListLobbies)
	s.HandleFunc("/lobbies/create", d.CreateLobby)
	log.Printf("Running server on port: %d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), r))

}

func (d Data) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL.Path)
		fmt.Println("Auth middleware")
		session, err := d.Sessions.Get(r, "session")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

		isAuthenticated := session.Values["authenticated"]
		if isAuthenticated != nil && isAuthenticated == true {
			fmt.Printf("User found %s", session.Values["user"])
			next.ServeHTTP(w, r)
		} else {
			// TODO: add better error handling
			http.Error(w, errors.New("{}").Error(), http.StatusUnauthorized)
		}

	})
}
