package persist

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// These are integration tests against a real MySQL instance (see
// compose.yaml's `db` service) rather than a mocked driver - a mock only
// proves the code calls Exec/Query with a string matching some regex, not
// that the SQL is actually correct against the real engine. Skips (not
// fails) if no test DB is reachable, so `go test ./...` still works without
// one; CI runs these for real (see .github/workflows/test-be.yml).
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost:3306"
	}
	cfg := mysql.Config{
		User:                 envOr("DB_USERNAME", "root"),
		Passwd:               envOr("DB_PASSWORD", "root_password"),
		Net:                  "tcp",
		Addr:                 host,
		DBName:               envOr("DB_NAME", "mishis4x"),
		AllowNativePasswords: true,
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Ping(); err != nil {
		t.Skipf("no test database reachable at %s, skipping integration test: %v", host, err)
	}

	return db
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestCreateAndFetchUser(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	username := fmt.Sprintf("test-user-%d", os.Getpid())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM users WHERE username = ?", username)
	})

	id, err := p.CreateUser(t.Context(), User{Username: username, Status: "active", Password: "hashedpw"})
	require.NoError(t, err)
	require.Greater(t, id, 0)

	byID, err := p.GetUserByID(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, username, byID.Username)
	require.Equal(t, "active", byID.Status)
	require.Equal(t, "hashedpw", byID.Password)
	require.Equal(t, id, byID.ID)

	byUsername, err := p.GetUserByUsername(t.Context(), username)
	require.NoError(t, err)
	require.Equal(t, byID, byUsername)
}

func TestGetUser_NotFound(t *testing.T) {
	db := testDB(t)
	p := &Persist{DB: db}

	missingUsername := fmt.Sprintf("does-not-exist-%d", os.Getpid())

	_, err := p.GetUserByUsername(t.Context(), missingUsername)
	require.ErrorIs(t, err, ErrUserNotFound)

	_, err = p.GetUserByID(t.Context(), -1)
	require.ErrorIs(t, err, ErrUserNotFound)
}
