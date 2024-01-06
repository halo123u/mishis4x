package cmd

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"example.com/mishis4x/persist"
	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var direction string
var seed bool
var env string

func init() {
	migrationsCMD.Flags().StringVarP(&direction, "direction", "d", "", "Direction of migrations (up or down)")
	migrationsCMD.Flags().BoolVarP(&seed, "seed", "s", false, "Seed the database")
	migrationsCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to run migrations on")
	rootCMD.AddCommand(migrationsCMD)
}

var migrationsCMD = &cobra.Command{
	Use:   "migrations",
	Short: "Run migrations",
	Long:  `Run migrations`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running migrations")

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
		}
		
		db, err := sql.Open("mysql", cfg.FormatDSN())
		if err != nil {
			log.Fatalf("failed to connect to %s",cfg.FormatDSN())
			panic(err)
		}

		if direction != "" {
			persist.RunMigrations(db, direction)
		}
		if seed {
			persist.SeedDB(db)
		}

	},
}
