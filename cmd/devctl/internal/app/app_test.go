package app_test

import (
	"testing"

	devctlapp "github.com/devctllabs/devctl/cmd/devctl/internal/app"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestCommandTreeHasDocumentationMetadata(t *testing.T) {
	t.Parallel()

	root := devctlapp.New("test", "")
	walkCommands(t, root, func(command *cli.Command) {
		require.NotEmpty(t, command.Usage, "%s has no usage", command.Path())
		require.NotEmpty(t, command.Description, "%s has no description", command.Path())

		for _, flag := range command.VisibleFlags() {
			documented, ok := flag.(cli.DocGenerationFlag)
			require.True(t, ok, "%s flag %v cannot be documented", command.Path(), flag.Names())
			require.NotEmpty(t, documented.GetUsage(), "%s flag %v has no usage", command.Path(), flag.Names())
		}

		for _, argument := range command.Arguments {
			require.NotEmpty(t, argument.Usage(), "%s has an undocumented argument", command.Path())
		}
	})
}

func walkCommands(t *testing.T, command *cli.Command, check func(*cli.Command)) {
	t.Helper()
	check(command)
	for _, child := range command.Commands {
		walkCommands(t, child, check)
	}
}
