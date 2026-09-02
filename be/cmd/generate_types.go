package cmd

import (
	"example.com/mishis4x/api"
	"example.com/mishis4x/logger"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tkrajina/typescriptify-golang-structs/typescriptify"
)

func init() {
	rootCMD.AddCommand(generateTypesCMD)
}

var generateTypesCMD = &cobra.Command{
	Use:   "generate-types",
	Short: "Regenerate fe/src/types.ts from the Go API structs",
	Long:  `Regenerate fe/src/types.ts from the Go API structs`,
	Run: func(cmd *cobra.Command, args []string) {
		// Dev/build-time tooling only, no --env of its own - always use
		// local's human-readable console output.
		logger.Init("local")
		generateTypes()
	},
}

func generateTypes() {
	log.Info().Msg("generating types")
	converter := typescriptify.New().Add(api.GlobalData{}).Add(api.Set{}).Add(api.Card{}).Add(api.AddOwnedSetInput{}).Add(api.OwnedCardInput{}).Add(api.SetOwnedCardsInput{}).Add(api.EbayListing{})

	err := converter.WithInterface(true).ConvertToFile("types.ts")
	if err != nil {
		log.Error().Err(err).Msg("error generating types")
	}
}
