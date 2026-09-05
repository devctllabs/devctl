package scaffold

// Command selects a project scaffold refresh.
type Command struct {
	ManifestPath string
}

// FileAction classifies the persisted effect on one scaffold file.
type FileAction string

const (
	FileCreated   FileAction = "created"
	FileUpdated   FileAction = "updated"
	FileUnchanged FileAction = "unchanged"
)

// FileChange records one scaffold file processed before success or failure.
type FileChange struct {
	// Path is relative to the project root.
	Path   string
	Action FileAction
}

// Result contains files completed before success or the first publication error.
type Result struct {
	Files []FileChange
}
