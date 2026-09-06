package command

import (
	"io"
	"os"
	"strings"

	loglib "github.com/devctllabs/go-libs/log"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// CommonCmdOpts contains the delivery options shared by every executable leaf.
type CommonCmdOpts struct {
	// ManifestPath selects a manifest explicitly; an empty value enables project discovery.
	ManifestPath string
	// JSON selects JSON events instead of the default console encoding.
	JSON bool
	// Verbose enables debug diagnostics and raw error causes.
	Verbose bool
}

// CommonFlags binds shared flags directly to the owning leaf's options.
func (o *CommonCmdOpts) CommonFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "file", Usage: "use `path` as the Manifest instead of discovering devctl.yaml", Destination: &o.ManifestPath},
		&cli.BoolFlag{Name: "json", Usage: "emit compact JSONL events instead of text", Destination: &o.JSON},
		&cli.BoolFlag{Name: "verbose", Usage: "include debug diagnostics and raw causes on stderr", Destination: &o.Verbose},
	}
}

// NewStdoutLogger creates the leaf result logger using the selected encoding.
func (o *CommonCmdOpts) NewStdoutLogger(command *cli.Command) *zap.Logger {
	return o.newLogger(command, outputWriter(command), zapcore.InfoLevel)
}

// NewStderrLogger creates the leaf diagnostic logger using the selected encoding and verbosity.
func (o *CommonCmdOpts) NewStderrLogger(command *cli.Command) *zap.Logger {
	level := zapcore.WarnLevel
	if o.Verbose {
		level = zapcore.DebugLevel
	}
	return o.newLogger(command, errorWriter(command), level)
}

func (o *CommonCmdOpts) newLogger(command *cli.Command, writer io.Writer, level zapcore.Level) *zap.Logger {
	encoding := loglib.EncodingConsole
	if o.JSON {
		encoding = loglib.EncodingJSON
	}
	logger := loglib.New(level, false, loglib.WithEncoding(encoding), loglib.WithOutput(writer))
	if name := leafCommandName(command); name != "" {
		logger = logger.With(zap.String("command", name))
	}
	return logger
}

func leafCommandName(command *cli.Command) string {
	path := command.Path()
	if len(path) > 0 && path[0] == "devctl" {
		path = path[1:]
	}
	return strings.Join(path, " ")
}

func outputWriter(command *cli.Command) io.Writer {
	if command.Writer != nil {
		return command.Writer
	}
	return os.Stdout
}

func errorWriter(command *cli.Command) io.Writer {
	if command.ErrWriter != nil {
		return command.ErrWriter
	}
	return os.Stderr
}
