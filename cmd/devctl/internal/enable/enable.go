package enable

import "github.com/urfave/cli/v3"

// NewCmd constructs the namespace for project capability commands.
func NewCmd() *cli.Command {
	return &cli.Command{
		Name:        "enable",
		Usage:       "Enable a project capability",
		Description: "Add or update one supported Capability in the Manifest. This command changes only devctl.yaml and does not refresh scaffold files or generated code.",
		Commands: []*cli.Command{
			newHTTPCmd(capabilityCmdOpts{}, buildCapability),
			newGRPCCmd(capabilityCmdOpts{}, buildCapability),
			newLoggingCmd(capabilityCmdOpts{}, buildCapability),
			newHealthCmd(capabilityCmdOpts{}, buildCapability),
			newTelemetryCmd(capabilityCmdOpts{}, buildCapability),
			newPprofCmd(capabilityCmdOpts{}, buildCapability),
		},
	}
}

func newGRPCCmd(opts capabilityCmdOpts, build capabilityBuilder) *cli.Command {
	return newCapabilityCmd("grpc", opts, build)
}
