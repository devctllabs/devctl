package enable

import "github.com/urfave/cli/v3"

// newHTTPCmd constructs the HTTP capability leaf.
func newHTTPCmd(opts capabilityCmdOpts, build capabilityBuilder) *cli.Command {
	return newCapabilityCmd("http", opts, build)
}
