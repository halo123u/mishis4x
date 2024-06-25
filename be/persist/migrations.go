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
	// todo figure out how to make this dynamic based on env
	sqlFilesDir := "./migrations/up"

	if direction == "down" {
		sqlFilesDir = "./migrations/down"
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

func SeedDB(db *sql.DB) {
	log.Println("Seeding database")
	sqlFilesDir := "./seeds"

	fileNames := []string{}
	err := filepath.Walk(sqlFilesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Panicf("failed reading file name: %s ",err)
	
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".sql") {
			fileNames = append(fileNames, path)
		}
		return nil
	})
	if err != nil {
		log.Panicf("failed reading file names: %s ",err)
	}

	for _, fileName := range fileNames {
		err := executeSQLFile(db, fileName)
		if err != nil {
			log.Printf("error executing %s: %v", fileName, err)
		} else {
			log.Printf("executed %s\n", fileName)
		}
	}
}

func executeSQLFile(db *sql.DB, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		log.Panicf("failed opening file: %s ",err)
	}
	sb := strings.Builder{}
	_, err = io.Copy(&sb, file)
	if err != nil {
		log.Panicf("failed copying file: %s ",err)
	}
	sql := sb.String()
	_, err = db.Exec(sql)
	if err != nil {
		log.Panicf("failed executing file: %s ",err)
	}

	return nil
}
