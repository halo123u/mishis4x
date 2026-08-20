package persist

import (
	"context"
	"errors"

	"github.com/rs/zerolog/log"
)

// ErrUserNotFound is returned by GetUserByID/GetUserByUsername when no row
// matches. Callers must check for it explicitly - it is not a query error.
var ErrUserNotFound = errors.New("user not found")

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
	found := false

	for stmt.Next() {
		err := stmt.Scan(&u.ID, &u.Username, &u.Status, &u.Password)
		if err != nil {
			return User{}, err
		}
		found = true
	}
	if !found {
		return User{}, ErrUserNotFound
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
	found := false

	for stmt.Next() {
		err := stmt.Scan(&u.Username, &u.Status, &u.Password, &u.ID)
		if err != nil {
			return User{}, err
		}
		found = true
	}
	if !found {
		return User{}, ErrUserNotFound
	}

	return u, nil
}

// UpdateUserPassword replaces userID's stored password hash. Callers must
// have already hashed newPassword (this function never sees plaintext) and
// verified the caller's identity - it does not.
func (p *Persist) UpdateUserPassword(ctx context.Context, userID int, hashedPassword string) error {
	q := `UPDATE users SET password = ? WHERE id = ?;`
	_, err := p.DB.ExecContext(ctx, q, hashedPassword, userID)
	return err
}
