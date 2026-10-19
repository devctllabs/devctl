package migrate

import "github.com/urfave/cli/v3"

func newCreateCmd(build migrateBuilder) *cli.Command {
	command, cmd := newLeaf("create", "Create a pair of timestamped SQL files", build)
	command.Arguments = []cli.Argument{&cli.StringArg{Name: "name", UsageText: "<name>", Destination: &cmd.opts.name}}
	return command
}
