package cmd

import (
	"os"
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the apiCmd.
func Execute() {
	err := apiCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
