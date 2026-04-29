package main

import (
	"embed"
	"io/fs"
)

//go:embed templates
var embeddedTemplates embed.FS

// templateFS is the embedded templates directory
// Paths within the FS are rooted at the templates directory: "install.sh", "install.ps1"
var templateFS = func() fs.FS {
	sub, err := fs.Sub(embeddedTemplates, "templates")
	if err != nil {
		panic("embedded templates directory not found: " + err.Error())
	}
	return sub
}()
