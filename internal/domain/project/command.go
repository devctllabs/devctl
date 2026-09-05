package project

// ChangeAction classifies the persisted effect of a manifest operation.
type ChangeAction string

const (
	ChangeCreated   ChangeAction = "created"
	ChangeUpdated   ChangeAction = "updated"
	ChangeUnchanged ChangeAction = "unchanged"
)

// ManifestResult identifies the selected manifest and its persisted change.
type ManifestResult struct {
	Manifest string
	Change   ChangeAction
}

// InitManifestCommand describes the canonical manifest to create or replace.
type InitManifestCommand struct {
	Destination string
	Language    string
	Preset      string
	Name        string
	Module      string
	Force       bool
}

// EnableCommand enables one project capability in an existing valid manifest.
type EnableCommand struct {
	ManifestPath string
	Capability   string
	Always       bool
	Force        bool
}

// AddDBCommand adds or replaces one database connection declaration.
type AddDBCommand struct {
	ManifestPath   string
	Name           string
	Kind           string
	Default        bool
	NoMigrations   bool
	MigrationsPath string
	Force          bool
}

// AddSourceCommand adds or replaces one local or external contract source.
type AddSourceCommand struct {
	ManifestPath      string
	Name              string
	Type              string
	Path              string
	URL               string
	Filename          string
	AllowInsecureHTTP bool
	Repo              string
	Ref               string
	BufConfig         string
	Force             bool
}

// AddHTTPClientCommand adds or replaces one generated HTTP client declaration.
type AddHTTPClientCommand struct {
	ManifestPath string
	Name         string
	Source       string
	Export       string
	Path         string
	BaseURLEnv   string
	Force        bool
}

type AddGRPCClientCommand struct {
	ManifestPath string
	Name         string
	Source       string
	Export       string
	Path         string
	ProtoRoot    string
	BufGenConfig string
	AddrEnv      string
	Force        bool
}

type AddKafkaConsumerCommand struct {
	ManifestPath                                                                      string
	Name, Topic, Source, Export, Path, Format, ProtoRoot, Message, Encoding, GroupEnv string
	Always, Force                                                                     bool
}

type AddKafkaProducerCommand struct {
	ManifestPath                                                                      string
	Name, Topic, Source, Export, Path, Format, ProtoRoot, Message, Encoding, TopicEnv string
	Force                                                                             bool
}

type AddRedisCommand struct {
	ManifestPath string
	Name         string
	AddrEnv      string
	AddrDefault  string
	Force        bool
}

type AddS3ConnectionCommand struct {
	ManifestPath string
	Name         string
	Credentials  string
	Force        bool
}

type AddS3Command struct {
	ManifestPath string
	Name         string
	Connection   string
	Force        bool
}
