package generate

import "github.com/devctllabs/devctl/internal/domain/artifact"

// Command selects generation targets and whether execution is a side-effect-free preview.
type Command struct {
	ManifestPath string
	Family       string
	Target       string
	DryRun       bool
}

// ChangeAction classifies an observed or planned managed-output change.
type ChangeAction string

const (
	ChangeCreated        ChangeAction = "created"
	ChangeUpdated        ChangeAction = "updated"
	ChangeUnchanged      ChangeAction = "unchanged"
	ChangeRemoved        ChangeAction = "removed"
	ChangePlannedPublish ChangeAction = "planned_publish"
	ChangePlannedRemove  ChangeAction = "planned_remove"
)

// Change records one generated managed-output decision.
type Change struct {
	Target string
	// Path is relative to the project root.
	Path   string
	Action ChangeAction
}

// Result contains targets completed before success or the first execution error.
type Result struct {
	Targets []string
	Changes []Change
	DryRun  bool
}

// Output separates an atomically published target directory from auxiliary project files.
type Output struct {
	Directory artifact.Tree
	Files     artifact.Tree
}
