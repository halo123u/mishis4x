package persist

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockPersist(t *testing.T) (*Persist, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return &Persist{DB: db}, mock
}

func TestCreateUser(t *testing.T) {
	p, mock := newMockPersist(t)

	mock.ExpectExec("INSERT INTO users").
		WithArgs("bilbo", "active", "hashedpw").
		WillReturnResult(sqlmock.NewResult(7, 1))

	id, err := p.CreateUser(User{Username: "bilbo", Status: "active", Password: "hashedpw"})

	require.NoError(t, err)
	assert.Equal(t, 7, id)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateUser_QueryError(t *testing.T) {
	p, mock := newMockPersist(t)

	mock.ExpectExec("INSERT INTO users").
		WillReturnError(errors.New("duplicate entry"))

	id, err := p.CreateUser(User{Username: "bilbo", Status: "active", Password: "hashedpw"})

	require.Error(t, err)
	assert.Equal(t, -1, id)
}

func TestGetUserByID(t *testing.T) {
	p, mock := newMockPersist(t)

	rows := sqlmock.NewRows([]string{"id", "username", "status", "password"}).
		AddRow(1, "frodo", "active", "hashedpw")
	mock.ExpectQuery("SELECT id, username, status, password").
		WithArgs(1).
		WillReturnRows(rows)

	u, err := p.GetUserByID(1)

	require.NoError(t, err)
	assert.Equal(t, User{ID: 1, Username: "frodo", Status: "active", Password: "hashedpw"}, u)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetUserByUsername(t *testing.T) {
	p, mock := newMockPersist(t)

	rows := sqlmock.NewRows([]string{"username", "status", "password", "id"}).
		AddRow("frodo", "active", "hashedpw", 1)
	mock.ExpectQuery("SELECT username, status, password, id").
		WithArgs("frodo").
		WillReturnRows(rows)

	u, err := p.GetUserByUsername("frodo")

	require.NoError(t, err)
	assert.Equal(t, User{ID: 1, Username: "frodo", Status: "active", Password: "hashedpw"}, u)
	assert.NoError(t, mock.ExpectationsWereMet())
}
