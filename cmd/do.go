package cmd

import (
	"github.com/ayushjn20/dunner/pkg/dunner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	rootCmd.AddCommand(doCmd)

	// Async Mode
	doCmd.Flags().BoolP("async", "A", false, "Asynchronous mode")
	if err := viper.BindPFlag("Async", doCmd.Flags().Lookup("async")); err != nil {
		log.Fatal(err)
	}

	// Dry-run mode
	doCmd.Flags().Bool("dry-run", false, "Dry-run of the command")
	if err := viper.BindPFlag("Dry-run", doCmd.Flags().Lookup("dry-run")); err != nil {
		log.Fatal(err)
	}

}

var doCmd = &cobra.Command{
	Use:   "do [taskName]",
	Short: "Do whatever you say",
	Long:  `You can run any task defined on the '.dunner.yaml' with this command`,
	Run:   dunner.Do,
	Args:  cobra.MinimumNArgs(1),
}
