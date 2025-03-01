package persist

type Todo struct {
	ID string
	Description string
	UserID string
	Completed bool
}


func (p *Persist) CreateTodo(t Todo) (Todo, error) {
	q := `
		INSERT INTO todos (id, description, user_id)
		VALUES (?, ?, ?);
	`
	_, err := p.DB.Exec(q, t.ID, t.Description, t.UserID)
	if err != nil {
		return Todo{}, err
	}

	todo, err := p.GetTodo(t.ID)
	if err != nil {
		return Todo{}, err
	}

	return todo, nil
}
func (p *Persist) GetTodo(id string) (Todo, error) {
	q := `
		SELECT id, description, user_id, completed
		FROM todos
		WHERE id = ?;
	`
	stmt, err := p.DB.Query(q, id)
	if err != nil {
		return Todo{}, err
	}

	defer stmt.Close()

	var t Todo

	for stmt.Next() {
		err := stmt.Scan(&t.ID, &t.Description, &t.UserID, &t.Completed)
		if err != nil {
			return Todo{}, err
		}
	}

	return t, nil
}