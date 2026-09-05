package gen

import "github.com/urfave/cli/v3"

// newGenHTTPCmd constructs the HTTP generation leaf.
func newGenHTTPCmd(opts genCmdOpts, build genBuilder) *cli.Command {
	return newGenLeaf(genLeafSpec{name: "http", family: "http", allowTarget: true}, opts, build)
}
