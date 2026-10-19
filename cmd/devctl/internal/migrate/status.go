package migrate

import "github.com/urfave/cli/v3"

func newStatusCmd(build migrateBuilder) *cli.Command {
	command, _ := newLeaf("status", "Show migration version and dirty state", build)
	command.Description = "Read the current migration version. Opening a fresh target may initialize the metadata table and SQLite database file. Dirty state is a successful result."
	return command
}
