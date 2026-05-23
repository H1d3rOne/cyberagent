package app

import "github.com/H1d3rOne/cyberagent/internal/interfaces/cli"

// Run starts the CyberAgent CLI application.
func Run(args []string) error {
	return RunWithVersion(args, Version)
}

// RunWithVersion starts the CLI with a caller-provided build/runtime version.
func RunWithVersion(args []string, version string) error {
	if version == "" {
		version = Version
	}
	root := cli.NewRootCommand(Name, version, Module)
	root.SetArgs(args)
	return root.Execute()
}
