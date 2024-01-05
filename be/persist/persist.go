package persist

import (
	"database/sql"
)

type Persist struct {
	DB *sql.DB
}
