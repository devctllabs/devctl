package enable

import "github.com/urfave/cli/v3"

// newPprofCmd constructs the pprof capability leaf.
func newPprofCmd(opts capabilityCmdOpts, build capabilityBuilder) *cli.Command {
	return newCapabilityCmd("pprof", opts, build)
}
