package persist

import (
	"context"
	"database/sql"
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
	// EmailAddress is nullable - existing/seeded accounts predate this
	// column. New accounts get it copied over from the invites row they
	// redeemed at signup (see handlers.UserCreate).
	EmailAddress *string
}

func (p *Persist) CreateUser(ctx context.Context, u User) (int, error) {
	q := `
		INSERT INTO users (username, status, password, email_address)
		VALUES (?, ?, ?, ?);
	`
	result, err := p.DB.ExecContext(ctx, q, u.Username, u.Status, u.Password, u.EmailAddress)
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
		SELECT id, username, status, password, email_address
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
	var email sql.NullString
	found := false

	for stmt.Next() {
		err := stmt.Scan(&u.ID, &u.Username, &u.Status, &u.Password, &email)
		if err != nil {
			return User{}, err
		}
		found = true
	}
	if !found {
		return User{}, ErrUserNotFound
	}
	if email.Valid {
		u.EmailAddress = &email.String
	}

	return u, nil
}

func (p *Persist) GetUserByUsername(ctx context.Context, username string) (User, error) {
	q := `
		SELECT username, status, password, id, email_address
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
	var email sql.NullString
	found := false

	for stmt.Next() {
		err := stmt.Scan(&u.Username, &u.Status, &u.Password, &u.ID, &email)
		if err != nil {
			return User{}, err
		}
		found = true
	}
	if !found {
		return User{}, ErrUserNotFound
	}
	if email.Valid {
		u.EmailAddress = &email.String
	}

	return u, nil
}

// GetUserByEmail looks up a user by email_address - used by the
// "forgot password" flow, where email (not username) is what the caller
// actually has. email_address is nullable (see User's own doc comment),
// so an account that predates that column - or was created before an
// email was ever attached - simply can't be found this way; that's
// surfaced to the caller identically to "no account has this email" (see
// handlers.RequestPasswordReset's doc comment for why that's the
// deliberately generic, non-enumerating response either way).
//
// email_address has no unique constraint (see its own migration's doc
// comment - nothing yet stops two separate invite requests from using the
// same address and both getting approved), so more than one account can
// share an address. Whoever calls this is deciding whose password to
// reset - not something safe to resolve by arbitrarily picking one of
// several matches - so more than one hit is treated the same as zero:
// ErrUserNotFound, not a guess.
func (p *Persist) GetUserByEmail(ctx context.Context, emailAddress string) (User, error) {
	q := `
		SELECT id, username, status, password, email_address
		FROM users
		WHERE email_address = ?;
	`
	stmt, err := p.DB.QueryContext(ctx, q, emailAddress)
	if err != nil {
		return User{}, err
	}

	defer func() {
		if closeErr := stmt.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("error closing statement")
		}
	}()

	var u User
	var email sql.NullString
	matchCount := 0

	for stmt.Next() {
		err := stmt.Scan(&u.ID, &u.Username, &u.Status, &u.Password, &email)
		if err != nil {
			return User{}, err
		}
		matchCount++
	}
	if matchCount != 1 {
		return User{}, ErrUserNotFound
	}
	if email.Valid {
		u.EmailAddress = &email.String
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
