package main

import (
	"fmt"
	"os"

	"github.com/thomaslaurenson/gpipe/cmd"
)

func main() {
	if err := cmd.Execute(templateFS); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
