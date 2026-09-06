package gen

import "github.com/urfave/cli/v3"

// newGenConfigCmd constructs the config-only generation leaf.
func newGenConfigCmd(opts genCmdOpts, build genBuilder) *cli.Command {
	return newGenLeaf(genLeafSpec{name: "config", family: "config"}, opts, build)
}
