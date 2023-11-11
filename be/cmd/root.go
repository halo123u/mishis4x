package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCMD = &cobra.Command{
	Use:   "be",
	Short: "Backend server for the game",
	Long:  `Backend server for the game`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Hello from the backend server")
	},
}

func Execute() {
	if err := rootCMD.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
