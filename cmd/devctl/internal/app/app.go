// Package app constructs the complete devctl command tree.
package app

import (
	"context"

	addcmd "github.com/devctllabs/devctl/cmd/devctl/internal/add"
	enablecmd "github.com/devctllabs/devctl/cmd/devctl/internal/enable"
	gencmd "github.com/devctllabs/devctl/cmd/devctl/internal/gen"
	initcmd "github.com/devctllabs/devctl/cmd/devctl/internal/init"
	inspectcmd "github.com/devctllabs/devctl/cmd/devctl/internal/inspect"
	lintcmd "github.com/devctllabs/devctl/cmd/devctl/internal/lint"
	synccmd "github.com/devctllabs/devctl/cmd/devctl/internal/sync"
	validatecmd "github.com/devctllabs/devctl/cmd/devctl/internal/validate"
	"github.com/urfave/cli/v3"
)

// New constructs the root command shared by the executable and documentation generator.
func New(releaseVersion, releaseCommit string) *cli.Command {
	return &cli.Command{
		Name:        "devctl",
		Usage:       "Manage Devctl Go projects",
		Description: "Devctl defines, validates, and materializes reproducible Go projects from a devctl.yaml manifest. Commands are non-interactive and keep manifest mutation, synchronization, linting, scaffolding, and generation explicit.",
		Version:     buildVersion(releaseVersion, releaseCommit),
		Commands: []*cli.Command{
			initcmd.NewCmd(),
			validatecmd.NewCmd(),
			inspectcmd.NewCmd(),
			enablecmd.NewCmd(),
			addcmd.NewCmd(),
			synccmd.NewCmd(),
			gencmd.NewCmd(),
			lintcmd.NewCmd(),
		},
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
		OnUsageError: func(_ context.Context, _ *cli.Command, err error, _ bool) error {
			return cli.Exit(err, 2)
		},
	}
}

func buildVersion(releaseVersion, releaseCommit string) string {
	if releaseCommit == "" {
		return releaseVersion
	}
	return releaseVersion + "\ncommit: " + releaseCommit
}
