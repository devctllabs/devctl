package internal

import (
	"context"
	"fmt"

	"example.test/sample-api/internal/deps"
	"github.com/urfave/cli/v3"
)

// NewCmdConsumer constructs the named Kafka consumer command.
func NewCmdConsumer() *cli.Command {
	return &cli.Command{
		Name:      "consumer",
		Arguments: []cli.Argument{&cli.StringArg{Name: "consumer-name"}},
		Action: func(ctx context.Context, command *cli.Command) error {
			scenario, err := deps.NewConsumer(ctx, command.StringArg("consumer-name"))
			if err != nil {
				return fmt.Errorf("deps.NewConsumer: %w", err)
			}
			if err := scenario.Run(ctx); err != nil {
				return fmt.Errorf("scenario.Run: %w", err)
			}
			return nil
		},
	}
}
