package persist

import (
	"context"

	"github.com/rs/zerolog/log"
)

type User struct {
	ID       int
	Username string
	Password string
	Status   string
}

func (p *Persist) CreateUser(ctx context.Context, u User) (int, error) {
	q := `
		INSERT INTO users (username, status, password)
		VALUES (?, ?, ?);
	`
	result, err := p.DB.ExecContext(ctx, q, u.Username, u.Status, u.Password)
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
func (p *Persist) GetUserByID(ctx context.Context, id int) (User, error) {
	q := `
		SELECT id, username, status, password
		FROM users
		WHERE id = ?;
	`
	stmt, err := p.DB.QueryContext(ctx, q, id)
	if err != nil {
		return User{}, err
	}

	defer func() {
		if closeErr := stmt.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing statement")
		}
	}()

	var u User

	for stmt.Next() {
		err := stmt.Scan(&u.ID, &u.Username, &u.Status, &u.Password)
		if err != nil {
			return User{}, err
		}
	}

	return u, nil
}

func (p *Persist) GetUserByUsername(ctx context.Context, username string) (User, error) {
	q := `
		SELECT username, status, password, id
		FROM users
		WHERE username = ?;
	`
	stmt, err := p.DB.QueryContext(ctx, q, username)
	if err != nil {
		return User{}, err
	}

	defer func() {
		if closeErr := stmt.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing statement")
		}
	}()

	var u User

	for stmt.Next() {
		err := stmt.Scan(&u.Username, &u.Status, &u.Password, &u.ID)
		if err != nil {
			return User{}, err
		}
	}

	return u, nil
}
