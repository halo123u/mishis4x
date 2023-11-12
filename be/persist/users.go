package persist

type User struct {
	ID       int
	Username string
	Password string
	Status   string
}

func (p *Persist) CreateUser(u User) (int, error) {
	q := `
		INSERT INTO users (username, status, password)
		VALUES (?, ?, ?);
	`
	result, err := p.DB.Exec(q, u.Username, u.Status, u.Password)
	if err != nil {
		return -1, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return -1, err
	}

	return int(id), nil
}

// TODO combine both into a query function
func (p *Persist) GetUserByID(id int) (User, error) {
	q := `
		SELECT id, username, status, password
		FROM users
		WHERE id = ?;
	`
	stmt, err := p.DB.Query(q, id)
	if err != nil {
		return User{}, err
	}

	defer stmt.Close()

	var u User

	for stmt.Next() {
		err := stmt.Scan(&u.ID, &u.Username, &u.Status, &u.Password)
		if err != nil {
			return User{}, err
		}
	}

	return u, nil
}

func (p *Persist) GetUserByUsername(username string) (User, error) {
	q := `
		SELECT username, status, password
		FROM users
		WHERE username = ?;
	`
	stmt, err := p.DB.Query(q, username)
	if err != nil {
		return User{}, err
	}

	defer stmt.Close()

	var u User

	for stmt.Next() {
		err := stmt.Scan(&u.Username, &u.Status, &u.Password)
		if err != nil {
			return User{}, err
		}
	}

	return u, nil
}
