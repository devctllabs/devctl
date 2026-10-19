package migration

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/failure"
	migrationdomain "github.com/devctllabs/devctl/internal/domain/migration"
	"github.com/golang-migrate/migrate/v4/source"
)

// Create reserves the timestamp and exclusively creates the two contained SQL files.
func (r *Repository) Create(ctx context.Context, creation migrationdomain.Creation) (result migrationdomain.Result, err error) {
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("ctx.Err: %w", err)
	}
	if !fs.ValidPath(creation.Target.Path) || !migrationdomain.ValidName(creation.Name) {
		return result, &migrationdomain.Error{Kind: failure.InvalidInput, Reason: "invalid_creation_path"}
	}
	root, err := os.OpenRoot(creation.Root)
	if err != nil {
		return result, migrationError(creation.Target.ID(), "source_open_failed", err, failure.Unavailable)
	}
	defer func() { err = errors.Join(err, closeError(creation.Target.ID(), root.Close())) }()
	if err := root.MkdirAll(creation.Target.Path, 0o755); err != nil {
		return result, migrationError(creation.Target.ID(), "directory_create_failed", err, failure.Unavailable)
	}
	version := r.now().UTC().Format("20060102150405")
	reservation := path.Join(creation.Target.Path, ".devctl-create-"+version)
	lock, err := root.OpenFile(reservation, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return result, migrationError(creation.Target.ID(), "version_reservation_failed", err, failure.Unavailable)
	}
	defer func() {
		err = errors.Join(err, closeError(creation.Target.ID(), errors.Join(lock.Close(), root.Remove(reservation))))
	}()
	if err := checkVersion(ctx, root.FS(), creation.Target.Path, version); err != nil {
		return result, migrationError(creation.Target.ID(), "version_conflict", err, failure.Unavailable)
	}
	for _, direction := range []string{"up", "down"} {
		name := path.Join(creation.Target.Path, version+"_"+creation.Name+"."+direction+".sql")
		if err := createEmptyFile(ctx, root, name); err != nil {
			cleanupErr := removeCreated(root, &result)
			return result, migrationError(creation.Target.ID(), "file_create_failed", errors.Join(err, cleanupErr), failure.Unavailable)
		}
		result.Files = append(result.Files, name)
	}
	result.Changed = true
	return result, nil
}

func checkVersion(ctx context.Context, files fs.FS, directory, version string) error {
	wanted, err := strconv.ParseUint(version, 10, 64)
	if err != nil {
		return fmt.Errorf("strconv.ParseUint: %w", err)
	}
	err = fs.WalkDir(files, directory, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("ctx.Err: %w", err)
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			return nil
		}
		migration, parseErr := source.Parse(entry.Name())
		if parseErr == nil && uint64(migration.Version) == wanted {
			return fmt.Errorf("migration version exists: %w", fs.ErrExist)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("fs.WalkDir: %w", err)
	}
	return nil
}

func createEmptyFile(ctx context.Context, root *os.Root, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("ctx.Err: %w", err)
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("root.OpenFile: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("file.Close: %w", errors.Join(err, root.Remove(name)))
	}
	return nil
}

func removeCreated(root *os.Root, result *migrationdomain.Result) error {
	var cleanupErr error
	var remaining []string
	for _, name := range result.Files {
		if err := root.Remove(name); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("root.Remove: %w", err))
			remaining = append(remaining, name)
		}
	}
	result.Files = remaining
	result.Changed = len(remaining) > 0
	return cleanupErr
}
