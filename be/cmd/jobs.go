package cmd

import (
	"fmt"

	"example.com/mishis4x/api"
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
		fmt.Println("Running jobs")
			fmt.Println(jobName)
		switch jobName {
		case "generate-types":
			generateTypes()
		}

	},
}

func generateTypes() {
	fmt.Println("Generating types")
	converter := typescriptify.New().Add(api.GlobalData{})

	err := converter.WithInterface(true).ConvertToFile("../fe/src/types.ts")
	if err != nil {
		fmt.Println(err)
	}

}