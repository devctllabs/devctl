package add

import (
	"context"
	"errors"
	"fmt"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	"github.com/devctllabs/devctl/internal/deps"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

type kafkaAdder interface {
	// AddKafkaConsumer adds or updates the Kafka consumer described by command.
	AddKafkaConsumer(ctx context.Context, command projectdomain.AddKafkaConsumerCommand) (projectdomain.ManifestResult, error)
	// AddKafkaProducer adds or updates the Kafka producer described by command.
	AddKafkaProducer(ctx context.Context, command projectdomain.AddKafkaProducerCommand) (projectdomain.ManifestResult, error)
}

type kafkaRuntime struct {
	adder    kafkaAdder
	shutdown func(context.Context) error
}

type kafkaBuilder func(context.Context, *zap.Logger) (kafkaRuntime, error)

type kafkaCmdOpts struct {
	commandruntime.CommonCmdOpts
	Name      string
	Topic     string
	Source    string
	Export    string
	Path      string
	Format    string
	ProtoRoot string
	Message   string
	Encoding  string
	GroupEnv  string
	TopicEnv  string
	Always    bool
	Force     bool
}

type kafkaCmd struct {
	opts         kafkaCmdOpts
	consumer     bool
	buildRuntime kafkaBuilder
}

func newKafkaConsumerCmd(opts kafkaCmdOpts, build kafkaBuilder) *cli.Command {
	return newKafkaCmd("kafka-consumer", true, opts, build)
}
func newKafkaProducerCmd(opts kafkaCmdOpts, build kafkaBuilder) *cli.Command {
	return newKafkaCmd("kafka-producer", false, opts, build)
}

func newKafkaCmd(name string, consumer bool, opts kafkaCmdOpts, build kafkaBuilder) *cli.Command {
	cmd := &kafkaCmd{opts: opts, consumer: consumer, buildRuntime: build}
	flags := append(cmd.opts.CommonFlags(),
		&cli.StringFlag{Name: "topic", Usage: "set the Kafka `topic`", Destination: &cmd.opts.Topic},
		&cli.StringFlag{Name: "source", Usage: "select the named contract `source`", Destination: &cmd.opts.Source},
		&cli.StringFlag{Name: "export", Usage: "select a named Export from a Devctl Source", Destination: &cmd.opts.Export},
		&cli.StringFlag{Name: "path", Usage: "select the schema Entrypoint from a non-Devctl Source", Destination: &cmd.opts.Path},
		&cli.StringFlag{Name: "format", Usage: "select `raw`, `json`, or `proto`", Destination: &cmd.opts.Format},
		&cli.StringFlag{Name: "proto-root", Usage: "set the Source-relative Proto `module-root`", Destination: &cmd.opts.ProtoRoot},
		&cli.StringFlag{Name: "message", Usage: "select the fully-qualified Proto `message`", Destination: &cmd.opts.Message},
		&cli.StringFlag{Name: "encoding", Usage: "select Proto `binary` or `json` encoding", Destination: &cmd.opts.Encoding},
	)
	if consumer {
		flags = append(flags,
			&cli.StringFlag{Name: "group-env", Usage: "override the consumer group environment `key`", Destination: &cmd.opts.GroupEnv},
			&cli.BoolFlag{Name: "always", Usage: "omit the Runtime Start Policy so the consumer is always enabled", Destination: &cmd.opts.Always},
		)
	} else {
		flags = append(flags, &cli.StringFlag{Name: "topic-env", Usage: "override the producer topic environment `key`", Destination: &cmd.opts.TopicEnv})
	}
	flags = append(flags, &cli.BoolFlag{Name: "force", Usage: "replace an existing Kafka endpoint with the same name", Destination: &cmd.opts.Force})
	return &cli.Command{
		Name:        name,
		Usage:       "Add a Kafka endpoint",
		Description: "Declare a named Kafka endpoint and its raw, JSON Schema, or Proto Contract. Schema-backed endpoints use --path for ordinary Sources or --export for Devctl Sources.",
		UsageText:   "devctl add " + name + " <" + name + "-name> --topic <topic> --format <raw|json|proto> [contract flags]",
		Arguments: []cli.Argument{&cli.StringArg{
			Name: name + "-name", UsageText: "<" + name + "-name>", Destination: &cmd.opts.Name,
		}},
		Flags:  flags,
		Action: cmd.Action,
	}
}

func (cmd *kafkaCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if cmd.opts.Name == "" {
		return reporter.ReportError(cli.Exit("Kafka endpoint name is required", 2))
	}
	if cmd.opts.Topic == "" {
		return reporter.ReportError(cli.Exit("--topic is required", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := cmd.add(ctx, runtime.adder)
	return finishManifestAddition(ctx, manifestAddition{stdout: stdout, reporter: reporter, shutdown: runtime.shutdown, result: result, operationErr: operationErr})
}

func (cmd *kafkaCmd) add(ctx context.Context, adder kafkaAdder) (projectdomain.ManifestResult, error) {
	if cmd.consumer {
		result, err := adder.AddKafkaConsumer(ctx, projectdomain.AddKafkaConsumerCommand{
			ManifestPath: cmd.opts.ManifestPath, Name: cmd.opts.Name, Topic: cmd.opts.Topic,
			Source: cmd.opts.Source, Export: cmd.opts.Export, Path: cmd.opts.Path,
			Format: cmd.opts.Format, ProtoRoot: cmd.opts.ProtoRoot, Message: cmd.opts.Message,
			Encoding: cmd.opts.Encoding, GroupEnv: cmd.opts.GroupEnv,
			Always: cmd.opts.Always, Force: cmd.opts.Force,
		})
		if err != nil {
			return result, fmt.Errorf("adder.AddKafkaConsumer: %w", err)
		}
		return result, nil
	}
	result, err := adder.AddKafkaProducer(ctx, projectdomain.AddKafkaProducerCommand{
		ManifestPath: cmd.opts.ManifestPath, Name: cmd.opts.Name, Topic: cmd.opts.Topic,
		Source: cmd.opts.Source, Export: cmd.opts.Export, Path: cmd.opts.Path,
		Format: cmd.opts.Format, ProtoRoot: cmd.opts.ProtoRoot, Message: cmd.opts.Message,
		Encoding: cmd.opts.Encoding, TopicEnv: cmd.opts.TopicEnv, Force: cmd.opts.Force,
	})
	if err != nil {
		return result, fmt.Errorf("adder.AddKafkaProducer: %w", err)
	}
	return result, nil
}

func buildKafka(ctx context.Context, logger *zap.Logger) (kafkaRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return kafkaRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.ProjectService()
	if err != nil {
		return kafkaRuntime{}, errors.Join(
			fmt.Errorf("container.ProjectService: %w", err),
			commandruntime.Shutdown(ctx, container),
		)
	}
	return kafkaRuntime{adder: service, shutdown: container.Shutdown}, nil
}
