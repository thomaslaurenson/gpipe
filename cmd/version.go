package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the gpipe version, injected at build time via
// -X github.com/thomaslaurenson/gpipe/cmd.Version. goreleaser supplies the
// release tag with the v stripped, so this is the unprefixed form; the
// v-prefixed form used in generated script headers is derived in internal.
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the gpipe version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("gpipe %s\n", Version)
	},
}
