package api

type Todo struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

type CreateTodoInput struct {
	Description string `json:"description"`
}