package add

import (
	"context"
	"errors"
	"time"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/go-libs/lifecycle"
	"go.uber.org/zap"
)

// manifestResultDTO is the stable payload shared by manifest resource additions.
type manifestResultDTO struct {
	Manifest string `json:"manifest"`
	Change   string `json:"change"`
}

// manifestAddition carries one mutation outcome through the shared shutdown boundary.
type manifestAddition struct {
	stdout       *zap.Logger
	reporter     *commandruntime.ErrorReporter
	shutdown     func(context.Context) error
	result       projectdomain.ManifestResult
	operationErr error
}

// finishManifestAddition joins operation and shutdown outcomes before emitting one final event.
func finishManifestAddition(ctx context.Context, addition manifestAddition) error {
	shutdownErr := lifecycle.Shutdown(ctx, 5*time.Second, addition.shutdown)
	dto := manifestResultDTO{Manifest: addition.result.Manifest, Change: string(addition.result.Change)}
	var options []commandruntime.ErrorOption
	if addition.result.Change != "" {
		options = append(options, commandruntime.WithPartialResult(dto))
	}
	if finalErr := errors.Join(addition.operationErr, shutdownErr); finalErr != nil {
		return addition.reporter.ReportError(finalErr, options...)
	}
	addition.stdout.Info("project resource addition completed", zap.Any("data", dto))
	return nil
}
