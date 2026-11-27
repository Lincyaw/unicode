package serve

import (
	"embed"
	"io/fs"
)

// ui holds the viewer's pages that the frontend's build wrote to ui/dist
// before go build (make build). ui/.gitkeep keeps the pattern matching when
// nothing was built, so a go build without Node still compiles.
//
//go:embed all:ui
var ui embed.FS

// Pages are the viewer's pages built into the program, or nil when it was
// built without them: ui/dist has no index.html.
func Pages() fs.FS {
	pages, err := fs.Sub(ui, "ui/dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(pages, "index.html"); err != nil {
		return nil
	}
	return pages
}
