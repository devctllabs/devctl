package enable

import "github.com/urfave/cli/v3"

// newHealthCmd constructs the health capability leaf.
func newHealthCmd(opts capabilityCmdOpts, build capabilityBuilder) *cli.Command {
	return newCapabilityCmd("health", opts, build)
}
