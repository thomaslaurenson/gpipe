package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	gpipe "github.com/thomaslaurenson/gpipe/internal"
)

var platformsCmd = &cobra.Command{
	Use:   "platforms",
	Short: "List supported platform identifiers",
	Long:  `Print all platform identifiers that can be used as keys in .gpipe.yml.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(strings.Join(gpipe.ValidPlatforms, "\n"))
	},
}
