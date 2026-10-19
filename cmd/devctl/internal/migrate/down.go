package migrate

import "github.com/urfave/cli/v3"

func newDownCmd(build migrateBuilder) *cli.Command {
	command, cmd := newLeaf("down", "Roll back migrations (one step by default)", build)
	command.Flags = append(command.Flags, &cli.IntFlag{Name: "steps", Value: 1, Usage: "roll back `N` migrations", Destination: &cmd.opts.steps})
	return command
}
