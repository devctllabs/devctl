package sync

// Command selects synchronization targets and whether execution is a side-effect-free preview.
type Command struct {
	ManifestPath string
	Family       string
	Target       string
	DryRun       bool
}

// ChangeAction classifies an observed or planned managed-contract change.
type ChangeAction string

const (
	ChangeCreated        ChangeAction = "created"
	ChangeUpdated        ChangeAction = "updated"
	ChangeUnchanged      ChangeAction = "unchanged"
	ChangeRemoved        ChangeAction = "removed"
	ChangePlannedPublish ChangeAction = "planned_publish"
	ChangePlannedRemove  ChangeAction = "planned_remove"
)

// Change records one managed-contract decision.
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
