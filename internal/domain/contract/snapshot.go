package contract

// Reference selects a contract entrypoint directly or through a named upstream export.
type Reference struct {
	Entrypoint string
	Export     string
	Format     string
	ProtoRoot  string
	Topic      string
}

// Location describes where a materialized contract can be resolved inside a project.
type Location struct {
	// Root is the containment boundary for RelativePath and Entrypoint.
	Root         string
	RelativePath string
	Entrypoint   string
	// Local distinguishes project-owned inputs from previously materialized managed output.
	Local bool
}

// File is one contract-closure file relative to its Snapshot root.
type File struct {
	Path    string
	Content []byte
	// Mode contains the source file's Unix permission bits.
	Mode uint32
}

// Snapshot is an exact local contract closure with Entrypoint naming one of Files.
type Snapshot struct {
	ModuleRoot string
	Entrypoint string
	Files      []File
	Metadata   *Metadata
}

// Metadata records upstream facts needed to detect stale materialized contracts.
type Metadata struct {
	Kind       string `json:"kind"`
	Topic      string `json:"topic,omitempty"`
	Format     string `json:"format,omitempty"`
	Entrypoint string `json:"entrypoint,omitempty"`
	ModuleRoot string `json:"module_root,omitempty"`
	BufConfig  string `json:"buf_config,omitempty"`
}
