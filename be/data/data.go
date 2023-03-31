package data

import (
	"database/sql"

	"example.com/mishis4x/lobbymodule"
	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type Data struct {
	DB *sql.DB
	L  *lobbymodule.Lobby
}

type User struct {
	Username string `json:"username"`
	ID       string `json:"user_id"`
	Status   string `json:"status"`
}

type NewUser struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Status   string `json:"status"`
}

type UserInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (d *Data) CreateUser(newUser UserInput) (User, error) {

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newUser.Password), bcrypt.DefaultCost)

	if err != nil {
		return User{}, err
	}

	nu := NewUser{
		Username: newUser.Username,
		Status:   "active",
		Password: string(hashedPassword),
	}
	u := User{}
	q := `
		INSERT INTO user (username, status, password)
		VALUES (?, ?, ?)
		RETURNING id, username, status
		`

	err = d.DB.QueryRow(q, nu.Username, nu.Status, string(nu.Password)).Scan(&u.ID, &u.Username, &u.Status)

	if err != nil {
		return User{}, err
	}

	return u, nil
}

func (d *Data) LoginUser(i UserInput) (User, bool, error) {

	q := `
		SELECT username, password, status, id 
		FROM user
		WHERE username = (?);
	`

	var savedPassword string
	u := User{}

	err := d.DB.QueryRow(q, i.Username).Scan(&u.Username, &savedPassword, &u.Status, &u.ID)

	if err != nil {
		return User{}, false, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(savedPassword), []byte(i.Password))

	if err != nil {
		return User{}, false, nil
	}

	return u, true, nil
}
