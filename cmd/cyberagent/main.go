package main

import (
	"fmt"
	"os"

	"github.com/H1d3rOne/cyberagent/internal/app"
)

var (
	version = app.Version
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("cyberagent %s (commit: %s, built: %s)\n", version, commit, date)
		return
	}
	if err := app.RunWithBuildInfo(os.Args[1:], app.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "cyberagent: %v\n", err)
		os.Exit(1)
	}
}
