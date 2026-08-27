package cmd

import (
	"example.com/mishis4x/api"
	"example.com/mishis4x/logger"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tkrajina/typescriptify-golang-structs/typescriptify"
)

var jobName string

func init() {
	jobsCMD.Flags().StringVarP(&jobName, "job", "j", "", "Name of the job to run")
	rootCMD.AddCommand(jobsCMD)
}

var jobsCMD = &cobra.Command{
	Use:   "jobs",
	Short: "Start the jobs server",
	Long:  `Start the jobs server`,
	Run: func(cmd *cobra.Command, args []string) {
		// No --env flag on this command; jobs are dev/build-time tooling, so
		// always use local's human-readable console output.
		logger.Init("local")
		log.Info().Str("job", jobName).Msg("running job")
		switch jobName {
		case "generate-types":
			generateTypes()
		}

	},
}

func generateTypes() {
	log.Info().Msg("generating types")
	converter := typescriptify.New().Add(api.GlobalData{}).Add(api.Set{}).Add(api.Card{})

	err := converter.WithInterface(true).ConvertToFile("types.ts")
	if err != nil {
		log.Error().Err(err).Msg("error generating types")
	}

}
