package cmd

import (
	"fmt"

	G "github.com/ayushjn20/dunner/pkg/global"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Dunner",
	Long:  `All software has versions. This is Dunners's`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(G.VERSION)
	},
}
