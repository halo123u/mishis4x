package persist

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

// connectTimeout bounds how long we'll wait to confirm the DB is actually
// reachable at startup. sql.Open never dials on its own - without this ping,
// an unreachable DB would only surface on the first real query.
const connectTimeout = 5 * time.Second

// tlsConfigName is the name configureTLS registers with the mysql driver
// for a CA-verified TLS connection - see configureTLS.
const tlsConfigName = "managed-mysql"

type Persist struct {
	DB *sql.DB
}

func NewDB(env string) (*sql.DB, error) {
	if env == "local" {
		envPath := "./infra/envs/local/.env"
		err := godotenv.Load(envPath)
		if err != nil {
			log.Fatal().Err(err).Msg("error loading .env file")
		}
	}

	dbUsername := os.Getenv("DB_USERNAME")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")

	// Deliberately not logging dbPassword.
	log.Debug().
		Str("dbUsername", dbUsername).
		Str("dbName", dbName).
		Str("dbHost", dbHost).
		Msg("connecting to db")

	cfg := mysql.Config{
		User:                 dbUsername,
		Passwd:               dbPassword,
		Net:                  "tcp",
		Addr:                 dbHost,
		DBName:               dbName,
		AllowNativePasswords: true,
		// Without this, the driver scans DATETIME/TIMESTAMP columns as raw
		// []byte instead of time.Time - fine as long as nothing scans into a
		// time.Time field, which stopped being true once sessions.expires_at
		// showed up.
		ParseTime: true,
	}

	if err := configureTLS(&cfg); err != nil {
		return nil, fmt.Errorf("configuring db TLS: %w", err)
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("pinging db: %w", err)
	}

	return db, nil
}

// configureTLS registers a custom TLS config trusting DB_CA_CERT (a base64-
// encoded PEM certificate) and points cfg at it, if DB_CA_CERT is set. Local/
// test's plain Docker MySQL has no TLS at all, so DB_CA_CERT is simply unset
// there and this is a no-op - only managed providers that require TLS with
// their own CA (DigitalOcean Managed Databases, etc.) need it set.
func configureTLS(cfg *mysql.Config) error {
	encoded := os.Getenv("DB_CA_CERT")
	if encoded == "" {
		return nil
	}

	pemBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("DB_CA_CERT is not valid base64: %w", err)
	}

	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(pemBytes); !ok {
		return errors.New("DB_CA_CERT does not contain a valid PEM certificate")
	}

	if err := mysql.RegisterTLSConfig(tlsConfigName, &tls.Config{RootCAs: pool}); err != nil {
		return fmt.Errorf("registering TLS config: %w", err)
	}

	cfg.TLSConfig = tlsConfigName
	return nil
}
