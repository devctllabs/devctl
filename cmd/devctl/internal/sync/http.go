package sync

import "github.com/urfave/cli/v3"

// newSyncHTTPCmd constructs the HTTP-only synchronization leaf.
func newSyncHTTPCmd(opts syncCmdOpts, build syncBuilder) *cli.Command {
	return newSyncLeaf("http", "http", opts, build)
}
