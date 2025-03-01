package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"example.com/mishis4x/api"
	"example.com/mishis4x/persist"
	"github.com/google/uuid"
)

const todosNamespace = "-tds"

func (d *Data) CreateTodo(w http.ResponseWriter, r *http.Request) {
	var cti api.CreateTodoInput
	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&cti)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}


	session, err := d.Sessions.Get(r, "session")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}


	input := persist.Todo{
		ID: uuid.NewString() + todosNamespace,
		Description: cti.Description,
		UserID: fmt.Sprintf("%d",session.Values["userID"].(int)),
	}

	todo, err := d.P.CreateTodo(input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(todo)
}