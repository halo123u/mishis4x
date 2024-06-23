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
	log.Printf("Running http server on port: %d\n", port)
	r := mux.NewRouter()
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

	r.PathPrefix(("/assets/")).Handler(http.StripPrefix("/assets/", http.FileServer(http.Dir("./dist/assets/"))))
  
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "./dist/index.html")
    })

	
	
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
			fmt.Printf("User found %s", session.Values["globalData"])
			next.ServeHTTP(w, r)
		} else if r.URL.Path == "/api/user/login" || r.URL.Path == "/api/user/create" {
			fmt.Println("Login or create user")
			next.ServeHTTP(w, r)
		} else {
			// TODO: add better error handling
			http.Error(w, errors.New("{}").Error(), http.StatusUnauthorized)
		}

	})
}
