package persist

import (
	"database/sql"
	"embed"
	"io/fs"
	"log"
	"slices"
	"strings"
)

func RunMigrations(db *sql.DB, direction string, files embed.FS) {
	log.Printf("Running %s migrations", direction)
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
			log.Printf("error executing %s: %v", fileName, err)
		} else {
			log.Printf("executed %s\n", fileName)
		}
	}

	log.Println("Migrations ran successfully")
}

func SeedDB(db *sql.DB, files embed.FS) {
	log.Println("Seeding database")
	sqlFilesDir := "seeds"

	fileNames := []string{}
	err := fs.WalkDir(files, sqlFilesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Panicf("failed reading file name: %s ", err)
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), ".sql") {
			fileNames = append(fileNames, path)
		}
		return nil
	})
	if err != nil {
		log.Panicf("failed reading file names: %s ", err)
	}

	for _, fileName := range fileNames {
		err := executeSQLFile(files, fileName, db)
		if err != nil {
			log.Printf("error executing %s: %v", fileName, err)
		} else {
			log.Printf("executed %s\n", fileName)
		}
	}
}

func executeSQLFile(files embed.FS, filePath string, db *sql.DB) error {
	content, err := files.ReadFile(filePath)
	if err != nil {
		log.Panicf("failed reading file: %s ", err)
	}

	_, err = db.Exec(string(content))
	if err != nil {
		log.Panicf("failed executing file: %s ", err)
	}

	return nil
}
