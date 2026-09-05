package project

// Manifest is the canonical desired project configuration before effective defaults are applied.
type Manifest struct {
	Version    int
	Project    Identity
	Env        Env
	Paths      ManifestPaths
	Sources    map[string]Source
	Exports    map[string]Export
	Components Components
	Languages  Languages
}

// Project binds one decoded Manifest to its selected filesystem root and manifest path.
type Project struct {
	Root         string
	ManifestPath string
	Manifest     Manifest
}

type Identity struct {
	Name     string
	Language string
}

// Export publishes a named contract path from an upstream Devctl project.
type Export struct {
	Kind     string
	Path     string
	Producer string
}

type Env struct {
	Prefix string
	Custom []EnvGroup
}

type EnvGroup struct {
	Group string
	Vars  []EnvVar
}

// EnvVar describes one generated environment variable declaration.
type EnvVar struct {
	Key  string
	Type string
	// Default retains the scalar type declared by the manifest.
	Default any
	Secret  bool
}

type ManifestPaths struct {
	ExternalContracts string
}

type Components struct {
	HTTP      *HTTP
	GRPC      *GRPC
	Kafka     *Kafka
	Logging   *Logging
	Health    *Health
	Telemetry *Telemetry
	DB        *DB
	Redis     *Redis
	S3        *S3
}

type Redis struct {
	Connections []RedisConnection
	Env         ComponentEnv
}

type RedisConnection struct {
	Name        string
	AddrEnv     string
	AddrDefault string
}

type S3 struct {
	Connections []S3Connection
	Buckets     []S3Bucket
	Env         ComponentEnv
}

type S3Connection struct {
	Name         string
	Credentials  string
	Endpoint     string
	Region       string
	PathStyle    bool
	AccessKeyEnv string
	SecretKeyEnv string
}

type S3Bucket struct {
	Name       string
	Connection string
	Bucket     string
}

type Kafka struct {
	Consumers []KafkaConsumer
	Producers []KafkaProducer
	Env       ComponentEnv
}

type KafkaContract struct {
	Source    string
	Export    string
	Path      string
	Format    string
	ProtoRoot string
	Message   string
	Encoding  string
}

type KafkaConsumer struct {
	Name     string
	Topic    string
	GroupEnv string
	Start    *Start
	Contract KafkaContract
}

type KafkaProducer struct {
	Name     string
	Topic    string
	TopicEnv string
	Contract KafkaContract
}

type GRPC struct {
	Server  *GRPCServer
	Clients []GRPCClient
	Env     ComponentEnv
}

type GRPCClient struct {
	Name         string
	Source       string
	Export       string
	Path         string
	ProtoRoot    string
	BufGenConfig string
	AddrEnv      string
}

type GRPCServer struct {
	ProtoRoot string
	BufConfig string
	Start     *Start
}

type ComponentEnv struct {
	System []EnvVar
	Custom []EnvVar
}

// Start controls whether a runtime component starts by default.
type Start struct {
	Env string
	// Default is nil when the manifest leaves the start policy unspecified.
	Default *bool
}

type HTTP struct {
	Server  *HTTPServer
	Clients []HTTPClient
	Env     ComponentEnv
}

type HTTPServer struct {
	OpenAPI string
	Start   *Start
}

// HTTPClient describes one generated client and its contract selection.
type HTTPClient struct {
	Name   string
	Source string
	Export string
	// Path is the contract entrypoint within Source when Export is not used.
	Path       string
	BaseURLEnv string
	// OAPIConfig is a project-relative oapi-codegen configuration path.
	OAPIConfig string
}

type Logging struct{ Env ComponentEnv }

type Health struct {
	Server *HealthServer
	Env    ComponentEnv
}

type HealthServer struct{ Start *Start }

type Telemetry struct {
	Start *Start
	Env   ComponentEnv
}

type DB struct {
	Connections []DBConnection
	Env         ComponentEnv
}

// DBConnection groups selectable variants of one logical database connection.
type DBConnection struct {
	Name string
	// Default names the variant selected when KindEnv is unset.
	Default  string
	KindEnv  string
	Variants []DBVariant
}

// DBVariant describes one concrete database driver and DSN source.
type DBVariant struct {
	Name       string
	Kind       string
	DSNEnv     string
	DSNDefault string
	Secret     bool
	Migrations *DBMigrations
}

// DBMigrations describes a golang-migrate target owned by one database variant.
type DBMigrations struct {
	Path            string
	DatabaseEnv     string
	DatabaseDefault string
}

type Languages struct{ Go GoLanguage }

type GoLanguage struct {
	Module     string
	Generators GoGenerators
	Components GoComponents
}

type GoGenerators struct {
	Config *ConfigGenerator
	HTTP   *HTTPGenerator
	GRPC   *GRPCGenerator
	Kafka  *KafkaGenerator
}

type KafkaGenerator struct {
	Out          string
	BufGenConfig string
}

type GRPCGenerator struct {
	Out          string
	BufGenConfig string
}

// ConfigGenerator configures the project-relative managed config output directory.
type ConfigGenerator struct{ Out string }

// HTTPGenerator configures oapi-codegen inputs and managed output directories.
type HTTPGenerator struct {
	OAPIConfig string
	ServerOut  string
	ClientOut  string
}

type GoComponents struct{ Pprof *Pprof }

type Pprof struct {
	Server *PprofServer
	Env    ComponentEnv
}

type PprofServer struct{ Start *Start }
