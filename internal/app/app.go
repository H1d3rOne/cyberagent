package app

import (
	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/cli"
)

// BuildInfo captures binary provenance for commands that write run snapshots.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Run starts the CyberAgent CLI application.
func Run(args []string) error {
	return RunWithBuildInfo(args, BuildInfo{Version: Version})
}

// RunWithVersion starts the CLI with a caller-provided build/runtime version.
func RunWithVersion(args []string, version string) error {
	return RunWithBuildInfo(args, BuildInfo{Version: version})
}

// RunWithBuildInfo starts the CLI with caller-provided build provenance.
func RunWithBuildInfo(args []string, build BuildInfo) error {
	version := build.Version
	if version == "" {
		version = Version
	}
	root := cli.NewRootCommandWithBuildInfo(Name, cliBuildInfo(version, build.Commit, build.Date))
	root.SetArgs(args)
	return root.Execute()
}

func cliBuildInfo(version, commit, date string) coreruntime.BuildInfo {
	return coreruntime.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
		Module:  Module,
	}
}
