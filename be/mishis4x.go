package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"github.com/julienschmidt/httprouter"

	_ "github.com/go-sql-driver/mysql"
)

type Handlers struct {
	db *sql.DB
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "Welcome!\n")
}

func Hello(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	fmt.Fprintf(w, "hello, %s!\n", ps.ByName("name"))
}

func main() {
	db, err := sql.Open("mysql", "user1:password@/mishis4x")
	if err != nil {
		panic(err)
	}

	h := Handlers{
		db: db,
	}

	router := httprouter.New()
	router.GET("/", h.Index)
	router.GET("/hello/:name", Hello)

	db.SetMaxOpenConns(10)

	log.Fatal(http.ListenAndServe(":8091", router))
}
