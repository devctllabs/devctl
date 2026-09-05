package manifest

type document struct {
	Version    int                       `yaml:"version"`
	Project    projectDocument           `yaml:"project"`
	Env        envDocument               `yaml:"env"`
	Paths      pathsDocument             `yaml:"paths"`
	Sources    map[string]sourceDocument `yaml:"sources"`
	Exports    map[string]exportDocument `yaml:"exports"`
	Components componentsDocument        `yaml:"components"`
	Languages  languagesDocument         `yaml:"languages"`
}

type projectDocument struct {
	Name     string `yaml:"name"`
	Language string `yaml:"language"`
}

type envDocument struct {
	Prefix string             `yaml:"prefix,omitempty"`
	Custom []envGroupDocument `yaml:"custom,omitempty"`
}

type envGroupDocument struct {
	Group string           `yaml:"group"`
	Vars  []envVarDocument `yaml:"vars"`
}

type envVarDocument struct {
	Key     string `yaml:"key"`
	Type    string `yaml:"type,omitempty"`
	Default any    `yaml:"default,omitempty"`
	Secret  bool   `yaml:"secret,omitempty"`
}

type pathsDocument struct {
	ExternalContracts string `yaml:"external_contracts,omitempty"`
}

type sourceDocument struct {
	Type              string              `yaml:"type"`
	Path              string              `yaml:"path,omitempty"`
	URL               string              `yaml:"url,omitempty"`
	Filename          string              `yaml:"filename,omitempty"`
	AllowInsecureHTTP bool                `yaml:"allow_insecure_http,omitempty"`
	Repo              string              `yaml:"repo,omitempty"`
	Ref               string              `yaml:"ref,omitempty"`
	Proto             sourceProtoDocument `yaml:"proto,omitempty"`
}

type sourceProtoDocument struct {
	BufConfig string `yaml:"buf_config,omitempty"`
}

type exportDocument struct {
	Kind     string `yaml:"kind"`
	Path     string `yaml:"path,omitempty"`
	Producer string `yaml:"producer,omitempty"`
}

type componentsDocument struct {
	HTTP      *httpDocument      `yaml:"http,omitempty"`
	GRPC      *grpcDocument      `yaml:"grpc,omitempty"`
	Kafka     *kafkaDocument     `yaml:"kafka,omitempty"`
	Logging   *loggingDocument   `yaml:"logging,omitempty"`
	Health    *healthDocument    `yaml:"health,omitempty"`
	Telemetry *telemetryDocument `yaml:"telemetry,omitempty"`
	DB        *dbDocument        `yaml:"db,omitempty"`
	Redis     *redisDocument     `yaml:"redis,omitempty"`
	S3        *s3Document        `yaml:"s3,omitempty"`
}

type redisDocument struct {
	Connections []redisConnectionDocument `yaml:"connections,omitempty"`
	Env         componentEnvDocument      `yaml:"env,omitempty"`
}

type redisConnectionDocument struct {
	Name        string `yaml:"name"`
	AddrEnv     string `yaml:"addr_env,omitempty"`
	AddrDefault string `yaml:"addr_default,omitempty"`
}

type s3Document struct {
	Connections []s3ConnectionDocument `yaml:"connections,omitempty"`
	Buckets     []s3BucketDocument     `yaml:"buckets,omitempty"`
	Env         componentEnvDocument   `yaml:"env,omitempty"`
}

type s3ConnectionDocument struct {
	Name         string `yaml:"name"`
	Credentials  string `yaml:"credentials,omitempty"`
	Endpoint     string `yaml:"endpoint,omitempty"`
	Region       string `yaml:"region,omitempty"`
	PathStyle    bool   `yaml:"path_style,omitempty"`
	AccessKeyEnv string `yaml:"access_key_env,omitempty"`
	SecretKeyEnv string `yaml:"secret_key_env,omitempty"`
}

type s3BucketDocument struct {
	Name       string `yaml:"name"`
	Connection string `yaml:"connection"`
	Bucket     string `yaml:"bucket,omitempty"`
}

type kafkaDocument struct {
	Consumers []kafkaConsumerDocument `yaml:"consumers,omitempty"`
	Producers []kafkaProducerDocument `yaml:"producers,omitempty"`
	Env       componentEnvDocument    `yaml:"env,omitempty"`
}

type kafkaContractDocument struct {
	Source    string `yaml:"source,omitempty"`
	Export    string `yaml:"export,omitempty"`
	Path      string `yaml:"path,omitempty"`
	Format    string `yaml:"format,omitempty"`
	ProtoRoot string `yaml:"proto_root,omitempty"`
	Message   string `yaml:"message,omitempty"`
	Encoding  string `yaml:"encoding,omitempty"`
}

type kafkaConsumerDocument struct {
	Name     string                `yaml:"name"`
	Topic    string                `yaml:"topic"`
	GroupEnv string                `yaml:"group_env,omitempty"`
	Start    *startDocument        `yaml:"start,omitempty"`
	Contract kafkaContractDocument `yaml:"contract,omitempty"`
}
type kafkaProducerDocument struct {
	Name     string                `yaml:"name"`
	Topic    string                `yaml:"topic"`
	TopicEnv string                `yaml:"topic_env,omitempty"`
	Contract kafkaContractDocument `yaml:"contract,omitempty"`
}

type grpcDocument struct {
	Server  *grpcServerDocument  `yaml:"server,omitempty"`
	Clients []grpcClientDocument `yaml:"clients,omitempty"`
	Env     componentEnvDocument `yaml:"env,omitempty"`
}

type grpcClientDocument struct {
	Name         string `yaml:"name"`
	Source       string `yaml:"source"`
	Export       string `yaml:"export,omitempty"`
	Path         string `yaml:"path,omitempty"`
	ProtoRoot    string `yaml:"proto_root,omitempty"`
	BufGenConfig string `yaml:"buf_gen_config,omitempty"`
	AddrEnv      string `yaml:"addr_env,omitempty"`
}

type grpcServerDocument struct {
	ProtoRoot string         `yaml:"proto_root,omitempty"`
	BufConfig string         `yaml:"buf_config,omitempty"`
	Start     *startDocument `yaml:"start,omitempty"`
}

type componentEnvDocument struct {
	System []envVarDocument `yaml:"system,omitempty"`
	Custom []envVarDocument `yaml:"custom,omitempty"`
}

type startDocument struct {
	Env     string `yaml:"env"`
	Default *bool  `yaml:"default,omitempty"`
}

type httpDocument struct {
	Server  *httpServerDocument  `yaml:"server,omitempty"`
	Clients []httpClientDocument `yaml:"clients,omitempty"`
	Env     componentEnvDocument `yaml:"env,omitempty"`
}

type httpServerDocument struct {
	OpenAPI string         `yaml:"openapi,omitempty"`
	Start   *startDocument `yaml:"start,omitempty"`
}

type httpClientDocument struct {
	Name       string `yaml:"name"`
	Source     string `yaml:"source"`
	Export     string `yaml:"export,omitempty"`
	Path       string `yaml:"path,omitempty"`
	BaseURLEnv string `yaml:"base_url_env,omitempty"`
	OAPIConfig string `yaml:"oapi_config,omitempty"`
}

type loggingDocument struct {
	Env componentEnvDocument `yaml:"env,omitempty"`
}

type healthDocument struct {
	Server *healthServerDocument `yaml:"server,omitempty"`
	Env    componentEnvDocument  `yaml:"env,omitempty"`
}

type healthServerDocument struct {
	Start *startDocument `yaml:"start,omitempty"`
}

type telemetryDocument struct {
	Start *startDocument       `yaml:"start,omitempty"`
	Env   componentEnvDocument `yaml:"env,omitempty"`
}

type dbDocument struct {
	Connections []dbConnectionDocument `yaml:"connections"`
	Env         componentEnvDocument   `yaml:"env,omitempty"`
}

type dbConnectionDocument struct {
	Name     string              `yaml:"name"`
	Default  string              `yaml:"default,omitempty"`
	KindEnv  string              `yaml:"kind_env,omitempty"`
	Variants []dbVariantDocument `yaml:"variants"`
}

type dbVariantDocument struct {
	Name       string                `yaml:"name"`
	Kind       string                `yaml:"kind"`
	DSNEnv     string                `yaml:"dsn_env,omitempty"`
	DSNDefault string                `yaml:"dsn_default,omitempty"`
	Secret     bool                  `yaml:"secret,omitempty"`
	Migrations *dbMigrationsDocument `yaml:"migrations,omitempty"`
}

type dbMigrationsDocument struct {
	Path            string `yaml:"path"`
	DatabaseEnv     string `yaml:"database_env"`
	DatabaseDefault string `yaml:"database_default,omitempty"`
}

type languagesDocument struct {
	Go goLanguageDocument `yaml:"go"`
}

type goLanguageDocument struct {
	Module     string               `yaml:"module"`
	Generators goGeneratorsDocument `yaml:"generators,omitempty"`
	Components goComponentsDocument `yaml:"components,omitempty"`
}

type goGeneratorsDocument struct {
	Config *configGeneratorDocument `yaml:"config,omitempty"`
	HTTP   *httpGeneratorDocument   `yaml:"http,omitempty"`
	GRPC   *grpcGeneratorDocument   `yaml:"grpc,omitempty"`
	Kafka  *kafkaGeneratorDocument  `yaml:"kafka,omitempty"`
}

type grpcGeneratorDocument struct {
	Out          string `yaml:"out,omitempty"`
	BufGenConfig string `yaml:"buf_gen_config,omitempty"`
}
type kafkaGeneratorDocument struct {
	Out          string `yaml:"out,omitempty"`
	BufGenConfig string `yaml:"buf_gen_config,omitempty"`
}

type configGeneratorDocument struct {
	Out string `yaml:"out,omitempty"`
}

type httpGeneratorDocument struct {
	OAPIConfig string `yaml:"oapi_config,omitempty"`
	ServerOut  string `yaml:"server_out,omitempty"`
	ClientOut  string `yaml:"client_out,omitempty"`
}

type goComponentsDocument struct {
	Pprof *pprofDocument `yaml:"pprof,omitempty"`
}

type pprofDocument struct {
	Server *pprofServerDocument `yaml:"server,omitempty"`
	Env    componentEnvDocument `yaml:"env,omitempty"`
}

type pprofServerDocument struct {
	Start *startDocument `yaml:"start,omitempty"`
}
