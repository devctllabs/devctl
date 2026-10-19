package migration

import "github.com/devctllabs/devctl/internal/domain/failure"

// Error retains a cause without rendering credentials or SQL in its message.
type Error struct {
	Kind       failure.Category
	Reason     string
	Target     string
	Path       string
	EnvKey     string
	Candidates []string
	Cause      error
}

func (e *Error) Error() string { return "migration failed: " + e.Reason }

func (e *Error) Unwrap() error { return e.Cause }

func (e *Error) Category() failure.Category { return e.Kind }
