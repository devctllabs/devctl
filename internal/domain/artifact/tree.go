package artifact

// File is one managed output file relative to its containing Tree.
type File struct {
	Path    string
	Content []byte
	// Mode contains Unix permission bits; publishers decide which other mode bits they support.
	Mode uint32
}

// Tree is the complete desired snapshot of one managed output directory.
type Tree struct {
	Files []File
}

// PublishAction classifies one atomically observed publication effect.
type PublishAction string

const (
	PublishCreated   PublishAction = "created"
	PublishUpdated   PublishAction = "updated"
	PublishUnchanged PublishAction = "unchanged"
	PublishRemoved   PublishAction = "removed"
)

// PublishChange describes one file inside a completely published Tree.
type PublishChange struct {
	Path   string
	Action PublishAction
}

// PublishResult reports the target effect and precise file effects from one publication call.
type PublishResult struct {
	Action  PublishAction
	Changes []PublishChange
}
