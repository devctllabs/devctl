// Package migration defines migration operations and their observable results.
package migration

import (
	"regexp"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

type Operation string

const (
	Create Operation = "create"
	Up     Operation = "up"
	Down   Operation = "down"
	Status Operation = "status"
)

// Command selects one Manifest migration target and operation.
type Command struct {
	ManifestPath string
	Target       string
	Operation    Operation
	EnvFile      string
	Name         string
	Steps        int // Up uses zero for all pending migrations; Up and Down use positive limits.
}

// State is the database's migration metadata; nil Version means no applied version.
type State struct {
	Version *uint
	Dirty   bool
}

// Result records observed state and files created by an operation.
type Result struct {
	Target   string
	State    *State
	Files    []string
	NoChange bool
	Changed  bool
}

var migrationName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// ValidName reports whether name is a safe migration filename component.
func ValidName(name string) bool { return migrationName.MatchString(name) }

// DatabaseConfig selects migration-only environment sources for one backend.
type DatabaseConfig struct {
	Kind    string
	EnvKey  string
	Default string
	EnvFile string
}

// Execution contains the resolved inputs to a database migration operation.
type Execution struct {
	Root        string
	Target      projectdomain.MigrationTarget
	Operation   Operation
	DatabaseURL string
	Steps       int // Up uses zero for all pending migrations; Up and Down use positive limits.
}

// Creation selects the contained directory and filename of a new SQL pair.
type Creation struct {
	Root   string
	Target projectdomain.MigrationTarget
	Name   string
}
