package cmd

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"example.com/mishis4x/persist"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var direction string

func init() {
	migrationsCMD.Flags().StringVarP(&direction, "direction", "d", "up", "Direction of migrations (up or down)")
	rootCMD.AddCommand(migrationsCMD)
}

var migrationsCMD = &cobra.Command{
	Use:   "migrations",
	Short: "Run migrations",
	Long:  `Run migrations`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running migrations")

		envPath := "./infra/envs/local/.env"
		err := godotenv.Load(envPath)

		if err != nil {
			log.Fatalf("error loading .env file: %v", err)
		}

		db, err := sql.Open("mysql", os.Getenv("DB_URL"))
		if err != nil {
			panic(err)
		}
		persist.RunMigrations(db, direction)

		if err != nil {
			panic(err)
		}
	},
}
