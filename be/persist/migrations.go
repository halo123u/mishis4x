package persist

import (
	"database/sql"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func RunMigrations(db *sql.DB, direction string) {
	log.Printf("Running %s migrations", direction)
	sqlFilesDir := "./persist/migrations/up"

	if direction == "down" {
		sqlFilesDir = "./persist/migrations/down"
	}
	fileNames := []string{}
	err := filepath.Walk(sqlFilesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			panic(err)
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".sql") {
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
		err := executeSQLFile(db, fileName)
		if err != nil {
			log.Printf("error executing %s: %v", fileName, err)
		} else {
			log.Printf("executed %s\n", fileName)
		}
	}

	log.Println("Migrations ran successfully")
}

func executeSQLFile(db *sql.DB, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}
	sb := strings.Builder{}
	_, err = io.Copy(&sb, file)
	if err != nil {
		panic(err)
	}
	sql := sb.String()
	_, err = db.Exec(sql)
	if err != nil {
		panic(err)
	}

	return nil
}
