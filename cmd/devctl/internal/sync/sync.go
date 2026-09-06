package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	"github.com/devctllabs/devctl/internal/deps"
	syncdomain "github.com/devctllabs/devctl/internal/domain/sync"
	"github.com/devctllabs/go-libs/lifecycle"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

//go:generate go tool mockgen -destination mocks/sync.go -package mocks -typed . syncer

type syncer interface {
	// Sync returns completed target facts even when a later target fails.
	Sync(ctx context.Context, command syncdomain.Command) (syncdomain.Result, error)
}

// syncRuntime contains the application port and cleanup hook used by Action.
type syncRuntime struct {
	syncer   syncer
	shutdown func(context.Context) error
}

// syncBuilder isolates dependency construction from synchronization behavior.
type syncBuilder func(ctx context.Context, logger *zap.Logger) (syncRuntime, error)

// syncCmd owns parsed options, a contract family selector, and its runtime factory.
type syncCmd struct {
	opts         syncCmdOpts
	family       string
	buildRuntime syncBuilder
}

// syncCmdOpts receives common flags plus synchronization target selection.
type syncCmdOpts struct {
	commandruntime.CommonCmdOpts
	Target string
	DryRun bool
}

// NewCmd constructs the sync command tree.
func NewCmd() *cli.Command {
	return newSyncCmd(syncCmdOpts{}, buildSync)
}

func newSyncCmd(opts syncCmdOpts, build syncBuilder) *cli.Command {
	command := newSyncLeaf("sync", "", opts, build)
	command.Commands = []*cli.Command{newSyncHTTPCmd(syncCmdOpts{}, build), newSyncLeaf("grpc", "grpc", syncCmdOpts{}, build), newSyncLeaf("kafka", "kafka", syncCmdOpts{}, build)}
	return command
}

// newSyncLeaf constructs one executable sync node for the selected contract family.
func newSyncLeaf(name, family string, opts syncCmdOpts, build syncBuilder) *cli.Command {
	cmd := &syncCmd{opts: opts, family: family, buildRuntime: build}
	usageText := "devctl sync"
	if family != "" {
		usageText += " " + name
	}
	flags := append(cmd.opts.CommonFlags(),
		&cli.StringFlag{Name: "target", Destination: &cmd.opts.Target, Usage: "select one exact Target `id`, such as http-client:billing"},
		&cli.BoolFlag{Name: "dry-run", Destination: &cmd.opts.DryRun, Usage: "preview publication and pruning without network access or writes"},
	)
	return &cli.Command{
		Name:        name,
		Usage:       "Synchronize external Contracts",
		Description: syncDescription(family),
		UsageText:   usageText + " [--target <target-id>] [--dry-run]",
		Flags:       flags,
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

func syncDescription(family string) string {
	if family == "" {
		return "Materialize every supported external Contract Snapshot into Project-owned paths. Full synchronization may prune stale Target directories; use --dry-run to preview changes."
	}
	return "Materialize external " + family + " Contract Snapshots. Family synchronization may prune stale Target directories; an explicit --target never prunes sibling Targets."
}

// Action synchronizes the selected contracts and emits the completed or partial result.
func (cmd *syncCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if command.Args().Len() != 0 {
		return reporter.ReportError(cli.Exit("sync accepts no positional arguments", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := runtime.syncer.Sync(ctx, syncdomain.Command{
		ManifestPath: cmd.opts.ManifestPath,
		Family:       cmd.family,
		Target:       cmd.opts.Target,
		DryRun:       cmd.opts.DryRun,
	})
	shutdownErr := lifecycle.Shutdown(ctx, 5*time.Second, runtime.shutdown)
	dto := syncDTO(result)
	if finalErr := errors.Join(operationErr, shutdownErr); finalErr != nil {
		var options []commandruntime.ErrorOption
		if !result.DryRun && (len(result.Targets) > 0 || len(result.Changes) > 0) {
			options = append(options, commandruntime.WithPartialResult(dto))
		}
		return reporter.ReportError(finalErr, options...)
	}
	stdout.Info("contract synchronization completed", zap.Any("data", dto))
	return nil
}

// syncChangeDTO is one stable contract publication fact.
type syncChangeDTO struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	Action string `json:"action"`
}

// syncResultDTO contains all completed targets and changes, including dry runs.
type syncResultDTO struct {
	Targets []string        `json:"targets"`
	Changes []syncChangeDTO `json:"changes"`
	DryRun  bool            `json:"dry_run"`
}

// syncDTO converts the domain result into the stable CLI payload.
func syncDTO(result syncdomain.Result) syncResultDTO {
	targets := make([]string, len(result.Targets))
	copy(targets, result.Targets)
	changes := make([]syncChangeDTO, 0, len(result.Changes))
	for _, change := range result.Changes {
		changes = append(changes, syncChangeDTO{Target: change.Target, Path: change.Path, Action: string(change.Action)})
	}
	return syncResultDTO{Targets: targets, Changes: changes, DryRun: result.DryRun}
}

// buildSync constructs and resolves the lazy dependencies owned by sync.
func buildSync(ctx context.Context, logger *zap.Logger) (syncRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return syncRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.SyncService()
	if err != nil {
		shutdownErr := commandruntime.Shutdown(ctx, container)
		return syncRuntime{}, errors.Join(fmt.Errorf("container.SyncService: %w", err), shutdownErr)
	}
	return syncRuntime{syncer: service, shutdown: container.Shutdown}, nil
}
