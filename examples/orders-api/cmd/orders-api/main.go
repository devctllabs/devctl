package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	appcmd "example.com/orders-api/cmd/orders-api/internal"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	root := &cli.Command{
		Name:  "orders-api",
		Usage: "Run orders-api",
		Commands: []*cli.Command{
			appcmd.NewCmdAPI(),
		},
	}
	if err := root.Run(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
