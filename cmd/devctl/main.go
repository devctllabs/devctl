package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	devctlapp "github.com/devctllabs/devctl/cmd/devctl/internal/app"
	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
)

var (
	version = "dev"
	commit  = ""
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	root := devctlapp.New(version, commit)

	if err := root.Run(ctx, os.Args); err != nil {
		rootOpts := commandruntime.CommonCmdOpts{}
		reporter := commandruntime.NewErrorReporter(rootOpts.NewStderrLogger(root), false)
		os.Exit(commandruntime.ExitCode(reporter.ReportError(err)))
	}
}
