package enable

import "github.com/urfave/cli/v3"

// newLoggingCmd constructs the logging capability leaf.
func newLoggingCmd(opts capabilityCmdOpts, build capabilityBuilder) *cli.Command {
	return newCapabilityCmd("logging", opts, build)
}
