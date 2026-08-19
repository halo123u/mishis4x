package cmd

import (
	"fmt"
	"log"

	dbassets "example.com/mishis4x/db"
	"example.com/mishis4x/persist"
	"github.com/spf13/cobra"
)

var direction string
var seed bool

func init() {
	migrationsCMD.Flags().StringVarP(&direction, "direction", "d", "", "Direction of migrations (up or down)")
	migrationsCMD.Flags().BoolVarP(&seed, "seed", "s" , false, "Seed the database")
	migrationsCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to run migrations on")
	rootCMD.AddCommand(migrationsCMD)
}

var migrationsCMD = &cobra.Command{
	Use:   "migrations",
	Short: "Run migrations",
	Long:  `Run migrations`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running migrations")

		db, err := persist.NewDB(env)
		if err != nil {
			log.Panicf("error connecting to db: %v", err)
		}

		if direction != "" {
			persist.RunMigrations(db, direction, dbassets.Files)
		}
		if seed {
			// Seed data is fake fixture data (e.g. a test user) - never run it
			// against a real environment's database.
			if env != "local" && env != "test" {
				log.Fatalf("refusing to seed data for env %q: seeding is only allowed for local/test", env)
			}
			persist.SeedDB(db, dbassets.Files)
		}

	},
}
