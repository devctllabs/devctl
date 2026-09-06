package initcmd

import "github.com/urfave/cli/v3"

// NewCmd constructs the namespace for project initialization commands.
func NewCmd() *cli.Command {
	return &cli.Command{
		Name:        "init",
		Usage:       "Initialize a Devctl project",
		Description: "Create the canonical Manifest or materialize the Go project foundation declared by an existing Manifest. Initialization steps are explicit and never run one another implicitly.",
		Commands: []*cli.Command{
			newManifestCmd(manifestCmdOpts{}, buildManifest),
			newScaffoldCmd(scaffoldCmdOpts{}, buildScaffold),
		},
	}
}
