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

func RunMigrations(db *sql.DB, direction string, files embed.FS) {
	log.Info().Str("direction", direction).Msg("running migrations")
	sqlFilesDir := "migrations/up"

	if direction == "down" {
		sqlFilesDir = "migrations/down"
	}
	fileNames := []string{}
	err := fs.WalkDir(files, sqlFilesDir, func(path string, d fs.DirEntry, err error) error {
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

	if direction == "down" {
		slices.Reverse(fileNames)
	}

	for _, fileName := range fileNames {
		err := executeSQLFile(files, fileName, db)
		if err != nil {
			log.Error().Err(err).Str("file", fileName).Msg("error executing migration file")
		} else {
			log.Info().Str("file", fileName).Msg("executed migration file")
		}
	}

	log.Info().Msg("migrations ran successfully")
}

func SeedDB(db *sql.DB, files embed.FS) {
	log.Info().Msg("seeding database")
	sqlFilesDir := "seeds"

	fileNames := []string{}
	err := fs.WalkDir(files, sqlFilesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatal().Err(err).Msg("failed reading seed file name")
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), ".sql") {
			fileNames = append(fileNames, path)
		}
		return nil
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed reading seed file names")
	}

	for _, fileName := range fileNames {
		err := executeSQLFile(files, fileName, db)
		if err != nil {
			log.Error().Err(err).Str("file", fileName).Msg("error executing seed file")
		} else {
			log.Info().Str("file", fileName).Msg("executed seed file")
		}
	}
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
