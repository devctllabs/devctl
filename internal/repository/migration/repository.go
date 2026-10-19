package migration

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	_ "github.com/ClickHouse/clickhouse-go" // Register the ClickHouse SQL driver used by its migration adapter.
	"github.com/devctllabs/devctl/internal/domain/failure"
	migrationdomain "github.com/devctllabs/devctl/internal/domain/migration"
	"github.com/devctllabs/go-libs/dbmigrate"
	devfs "github.com/devctllabs/go-libs/filesystem"
	upstream "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/clickhouse"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source"
)

// Repository owns migration database and contained SQL-file mechanics.
type Repository struct{ now func() time.Time }

func NewRepository() *Repository { return &Repository{now: time.Now} }

// Execute keeps the contained source alive until the migration database is closed.
func (r *Repository) Execute(ctx context.Context, execution migrationdomain.Execution) (result migrationdomain.Result, err error) {
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("ctx.Err: %w", err)
	}
	target := execution.Target.ID()
	disk, err := devfs.Open(execution.Root)
	if err != nil {
		return result, migrationError(target, "source_open_failed", err, failure.Unavailable)
	}
	defer func() { err = errors.Join(err, closeError(target, disk.Close())) }()
	migrator, err := dbmigrate.New(disk, dbmigrate.Config{DatabaseURL: execution.DatabaseURL, Directories: []string{execution.Target.Path}})
	if err != nil {
		return result, migrationError(target, "migration_open_failed", err, failure.Unavailable)
	}
	defer func() { err = errors.Join(err, closeError(target, migrator.Close())) }()
	return runMigration(ctx, migrator, execution)
}

func runMigration(ctx context.Context, migrator *dbmigrate.Migrator, execution migrationdomain.Execution) (migrationdomain.Result, error) {
	var result migrationdomain.Result
	target := execution.Target.ID()
	before, err := readState(migrator)
	if err != nil {
		return result, migrationError(target, "version_read_failed", err, failure.Unavailable)
	}
	var operationErr error
	switch execution.Operation {
	case migrationdomain.Status:
		result.State = before
		return result, nil
	case migrationdomain.Up:
		if execution.Steps > 0 {
			operationErr = migrator.UpN(ctx, execution.Steps)
		} else {
			operationErr = migrator.Up(ctx)
		}
	case migrationdomain.Down:
		operationErr = migrator.Down(ctx, execution.Steps)
	case migrationdomain.Create:
		operationErr = &migrationdomain.Error{Kind: failure.Unsupported, Reason: "unsupported_operation"}
	default:
		operationErr = &migrationdomain.Error{Kind: failure.Unsupported, Reason: "unsupported_operation"}
	}
	if execution.Operation == migrationdomain.Up && errors.Is(operationErr, upstream.ErrNoChange) {
		operationErr = nil
		result.NoChange = true
	}
	state, stateErr := readState(migrator)
	result.State = state
	completedChange := operationErr == nil && !result.NoChange
	observedChange := state != nil && !sameState(before, state)
	result.Changed = completedChange || observedChange
	err = errors.Join(operationErr, stateErr)
	if err != nil {
		return result, migrationError(target, "migration_execution_failed", err, failure.Internal)
	}
	return result, nil
}

func readState(migrator *dbmigrate.Migrator) (*migrationdomain.State, error) {
	version, dirty, err := migrator.Version()
	if errors.Is(err, upstream.ErrNilVersion) {
		return &migrationdomain.State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("migrator.Version: %w", err)
	}
	return &migrationdomain.State{Version: &version, Dirty: dirty}, nil
}

func sameState(left, right *migrationdomain.State) bool {
	if left.Dirty != right.Dirty {
		return false
	}
	if left.Version == nil || right.Version == nil {
		return left.Version == nil && right.Version == nil
	}
	return *left.Version == *right.Version
}

func closeError(target string, err error) error {
	if err == nil {
		return nil
	}
	return migrationError(target, "close_failed", err, failure.Unavailable)
}

func migrationError(target, reason string, cause error, fallback failure.Category) error {
	kind := fallback
	var dirty upstream.ErrDirty
	var duplicate source.ErrDuplicateMigration
	var shortage upstream.ErrShortLimit
	switch {
	case errors.As(cause, &dirty), errors.As(cause, &duplicate), errors.Is(cause, fs.ErrExist), errors.Is(cause, upstream.ErrLocked), errors.Is(cause, upstream.ErrLockTimeout):
		kind = failure.Conflict
	case errors.Is(cause, fs.ErrNotExist):
		kind = failure.NotFound
	case errors.As(cause, &shortage), errors.Is(cause, dbmigrate.ErrInvalidSteps), errors.Is(cause, dbmigrate.ErrInvalidConfig), errors.Is(cause, fs.ErrInvalid):
		kind = failure.InvalidInput
	}
	return &migrationdomain.Error{Kind: kind, Reason: reason, Target: target, Cause: cause}
}
