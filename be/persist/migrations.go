package persist

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// statementTimeout bounds how long a single migration/seed SQL file is
// allowed to run. These run as a one-shot process at deploy time (see
// cmd/migrations.go), not per-request, so this is generous compared to
// request-scoped DB timeouts.
const statementTimeout = 30 * time.Second

// RunMigrations applies every "up" (or reverses every "down") SQL file in
// files, tracking what's already been applied in a schema_migrations table
// so it's safe to call repeatedly against a DB that already has the schema -
// e.g. every `docker compose up` against a persisted volume, not just a
// fresh one. Without this, a second run would try to CREATE TABLE on tables
// that already exist and hard-fail (executeSQLFile calls log.Fatal on any
// SQL error).
func RunMigrations(db *sql.DB, direction string, files embed.FS) {
	log.Info().Str("direction", direction).Msg("running migrations")

	ensureSchemaMigrationsTable(db)

	sqlFilesDir := "migrations/up"
	if direction == "down" {
		sqlFilesDir = "migrations/down"
	}
	fileNames := listSQLFiles(files, sqlFilesDir)

	if direction == "down" {
		slices.Reverse(fileNames)
	}

	for _, fileName := range fileNames {
		if direction == "up" {
			applied, err := migrationApplied(db, fileName)
			if err != nil {
				log.Fatal().Err(err).Str("file", fileName).Msg("error checking migration status")
			}
			if applied {
				log.Debug().Str("file", fileName).Msg("already applied, skipping")
				continue
			}
		}

		if err := executeSQLFile(files, fileName, db); err != nil {
			log.Error().Err(err).Str("file", fileName).Msg("error executing migration file")
			continue
		}
		log.Info().Str("file", fileName).Msg("executed migration file")

		if direction == "up" {
			if err := recordMigration(db, fileName); err != nil {
				log.Fatal().Err(err).Str("file", fileName).Msg("error recording migration")
			}
		}
	}

	if direction == "down" {
		// A full down run reverses everything, so tracking is reset wholesale
		// rather than trying to map individual down files back to the up
		// file they reverse (the two don't share a filename to correlate by -
		// see the migrations/ directory's naming).
		if err := clearMigrationRecords(db); err != nil {
			log.Fatal().Err(err).Msg("error clearing migration records")
		}
	}

	log.Info().Msg("migrations ran successfully")
}

// SeedDB inserts fixture data, tracked the same way as RunMigrations - safe
// to call on every startup against a persisted DB without erroring on
// already-seeded data or (worse, for tables with no unique constraint on the
// fixture row) silently duplicating it.
func SeedDB(db *sql.DB, files embed.FS) {
	log.Info().Msg("seeding database")

	ensureSchemaMigrationsTable(db)

	fileNames := listSQLFiles(files, "seeds")

	for _, fileName := range fileNames {
		applied, err := migrationApplied(db, fileName)
		if err != nil {
			log.Fatal().Err(err).Str("file", fileName).Msg("error checking seed status")
		}
		if applied {
			log.Debug().Str("file", fileName).Msg("already seeded, skipping")
			continue
		}

		if err := executeSQLFile(files, fileName, db); err != nil {
			log.Error().Err(err).Str("file", fileName).Msg("error executing seed file")
			continue
		}
		log.Info().Str("file", fileName).Msg("executed seed file")

		if err := recordMigration(db, fileName); err != nil {
			log.Fatal().Err(err).Str("file", fileName).Msg("error recording seed")
		}
	}
}

func listSQLFiles(files embed.FS, dir string) []string {
	fileNames := []string{}
	err := fs.WalkDir(files, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			panic(err)
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".sql") {
			fileNames = append(fileNames, path)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return fileNames
}

func executeSQLFile(files embed.FS, filePath string, db *sql.DB) error {
	content, err := files.ReadFile(filePath)
	if err != nil {
		log.Fatal().Err(err).Str("file", filePath).Msg("failed reading sql file")
	}

	ctx, cancel := context.WithTimeout(context.Background(), statementTimeout)
	defer cancel()

	_, err = db.ExecContext(ctx, string(content))
	if err != nil {
		log.Fatal().Err(err).Str("file", filePath).Msg("failed executing sql file")
	}

	return nil
}

// ensureSchemaMigrationsTable creates the tracking table if it doesn't
// already exist. Deliberately not itself a tracked migration file - it has
// to exist before anything can check what's already been applied.
func ensureSchemaMigrationsTable(db *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), statementTimeout)
	defer cancel()

	q := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) NOT NULL PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`
	if _, err := db.ExecContext(ctx, q); err != nil {
		log.Fatal().Err(err).Msg("failed ensuring schema_migrations table exists")
	}
}

func migrationApplied(db *sql.DB, version string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), statementTimeout)
	defer cancel()

	var exists bool
	q := `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?);`
	err := db.QueryRowContext(ctx, q, version).Scan(&exists)
	return exists, err
}

func recordMigration(db *sql.DB, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), statementTimeout)
	defer cancel()

	_, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?);`, version)
	return err
}

func clearMigrationRecords(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), statementTimeout)
	defer cancel()

	_, err := db.ExecContext(ctx, `DELETE FROM schema_migrations;`)
	return err
}
