package cmd

import (
	"database/sql"
	"log"
	"os"

	"example.com/mishis4x/handlers"
	"example.com/mishis4x/matchmaking"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

func init() {
	rootCMD.AddCommand(httpCMD)
}

var httpCMD = &cobra.Command{
	Use:   "http",
	Short: "Start the HTTP server",
	Long:  `Start the HTTP server`,
	Run: func(cmd *cobra.Command, args []string) {
		envPath := "./infra/envs/local/.env"
		err := godotenv.Load(envPath)
		if err != nil {
			log.Fatalf("error loading .env file: %v", err)
		}

		db, err := sql.Open("mysql", os.Getenv("DB_URL"))
		if err != nil {
			panic(err)
		}

		db.SetMaxOpenConns(5)
		port := 8091

		h := handlers.Data{
			DB: db,
			Lobby: &matchmaking.Lobby{
				Games:  []*matchmaking.Game{},
				GameID: 1,
			},
		}
		h.InitializeHttpServer(port)

	}}
