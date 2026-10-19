package migrate

import "github.com/urfave/cli/v3"

func newUpCmd(build migrateBuilder) *cli.Command {
	command, cmd := newLeaf("up", "Apply all pending migrations, or a positive number with --steps", build)
	command.Flags = append(command.Flags, &cli.IntFlag{Name: "steps", Usage: "apply positive `N` migrations; omit for all", Destination: &cmd.opts.steps})
	return command
}
