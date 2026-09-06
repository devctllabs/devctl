package lint

import "github.com/urfave/cli/v3"

// newLintHTTPCmd constructs the HTTP-only lint leaf.
func newLintHTTPCmd(opts lintCmdOpts, build lintBuilder) *cli.Command {
	return newLintLeaf("http", "http", opts, build)
}
