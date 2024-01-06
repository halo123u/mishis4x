package persist

import (
	"database/sql"
	"log"
	"os"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type Persist struct {
	DB *sql.DB
}


func NewDB(env string) (*sql.DB, error) {
	if env == "local" {
		envPath := "./infra/envs/local/.env"
		err := godotenv.Load(envPath)
		if err != nil {
			log.Fatalf("error loading .env file: %v", err)
		}
	}	

	dbUsername := os.Getenv("DB_USERNAME")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")

	cfg := mysql.Config{
		User:                 dbUsername,
		Passwd:               dbPassword,
		Net:                  "tcp",
		Addr:                 dbHost,
		DBName:               dbName,
		AllowNativePasswords: true,
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}

	return db, nil
}