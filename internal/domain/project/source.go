package project

// SourceType identifies the mechanism used to obtain a contract.
type SourceType string

const (
	SourceLocal  SourceType = "local"
	SourceURL    SourceType = "url"
	SourceGit    SourceType = "git"
	SourceDevctl SourceType = "devctl"
)

// Source describes a named local or external origin of contracts.
type Source struct {
	Name              string
	Type              SourceType
	Path              string
	URL               string
	Filename          string
	AllowInsecureHTTP bool
	Repo              string
	Ref               string
	Proto             SourceProto
}

// SourceProto contains protobuf tooling metadata relative to the source root.
type SourceProto struct {
	BufConfig string
}
