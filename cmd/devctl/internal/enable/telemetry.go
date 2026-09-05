package enable

import "github.com/urfave/cli/v3"

// newTelemetryCmd constructs the telemetry capability leaf.
func newTelemetryCmd(opts capabilityCmdOpts, build capabilityBuilder) *cli.Command {
	return newCapabilityCmd("telemetry", opts, build)
}
