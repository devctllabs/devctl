package project

// InspectQuery selects the manifest whose effective project view is requested.
type InspectQuery struct {
	ManifestPath string
}

// InspectResult contains the effective view derived from one valid manifest.
type InspectResult struct {
	Project Inspection
}

// Inspection is the effective view of one selected manifest-managed directory.
type Inspection struct {
	Root         string
	ManifestPath string
	Name         string
	Language     string
	Module       string
	EnvPrefix    string
	Paths        Paths
	Targets      []InspectionTarget
	Env          []EffectiveEnv
	Resources    InspectionResources
}

// InspectionTarget describes one effective contract or config generation target.
type InspectionTarget struct {
	ID            string
	Family        string
	Format        string
	Input         string
	ResolvedInput string
	Config        string
	Output        string
}

// EffectiveEnv describes one fully-prefixed environment variable.
type EffectiveEnv struct {
	Key     string
	Type    string
	Default any
	Secret  bool
}

// InspectionResources inventories named runtime resources.
type InspectionResources struct {
	DBConnections    []string
	RedisConnections []string
	S3Connections    []string
	S3Buckets        []string
	Migrations       []InspectionMigration
}

// InspectionMigration is one effective golang-migrate target.
type InspectionMigration struct {
	Connection  string
	Variant     string
	Kind        string
	Path        string
	DatabaseEnv string
}

// Paths contains effective project-relative managed-output locations.
type Paths struct {
	ExternalContracts string
	ConfigOut         string
	ServerOut         string
	ClientOut         string
}

// ValidateQuery selects the manifest whose project readiness should be checked.
type ValidateQuery struct {
	ManifestPath string
}

// ValidationResult reports all project readiness issues that could be collected.
// Invalid project data is a normal result rather than an execution error.
type ValidationResult struct {
	Issues []Issue
}

// IsValid reports whether validation found no issues.
func (r ValidationResult) IsValid() bool {
	return len(r.Issues) == 0
}

// IssueCode identifies one stable project validation rule.
type IssueCode string

const (
	IssueYAMLInvalid            IssueCode = "yaml_invalid"
	IssueSchemaInvalid          IssueCode = "schema_invalid"
	IssueYAMLDuplicateKey       IssueCode = "yaml_duplicate_key"
	IssueSchemaUnknownField     IssueCode = "schema_unknown_field"
	IssueVersionUnsupported     IssueCode = "version_unsupported"
	IssueNameInvalid            IssueCode = "name_invalid"
	IssueLanguageUnsupported    IssueCode = "language_unsupported"
	IssueGoModuleRequired       IssueCode = "go_module_required"
	IssuePathInvalid            IssueCode = "path_invalid"
	IssuePathOverlap            IssueCode = "path_overlap"
	IssueSourceNameInvalid      IssueCode = "source_name_invalid"
	IssueSourceInvalid          IssueCode = "source_invalid"
	IssueSourceInsecure         IssueCode = "source_insecure"
	IssueSourceTypeUnsupported  IssueCode = "source_type_unsupported"
	IssueExportInvalid          IssueCode = "export_invalid"
	IssueHTTPClientInvalid      IssueCode = "http_client_invalid"
	IssueGRPCClientInvalid      IssueCode = "grpc_client_invalid"
	IssueKafkaContractInvalid   IssueCode = "kafka_contract_invalid"
	IssueSourceNotFound         IssueCode = "source_not_found"
	IssueDBConnectionInvalid    IssueCode = "db_connection_invalid"
	IssueDBVariantInvalid       IssueCode = "db_variant_invalid"
	IssueDBDefaultInvalid       IssueCode = "db_default_invalid"
	IssueDBMigrationsInvalid    IssueCode = "db_migrations_invalid"
	IssueMigrationPathMissing   IssueCode = "migration_path_missing"
	IssueRedisConnectionInvalid IssueCode = "redis_connection_invalid"
	IssueRedisAddressInvalid    IssueCode = "redis_address_invalid"
	IssueS3ConnectionNotFound   IssueCode = "s3_connection_not_found"
	IssueGoModMissing           IssueCode = "go_mod_missing"
	IssueGoModInvalid           IssueCode = "go_mod_invalid"
	IssueSourceMissing          IssueCode = "source_missing"
	IssueOpenAPIMissing         IssueCode = "openapi_missing"
	IssueHTTPGeneratorMissing   IssueCode = "http_generator_missing"
	IssueToolConfigMissing      IssueCode = "tool_config_missing"
	IssueToolConfigInvalid      IssueCode = "tool_config_invalid"
	IssueToolMissing            IssueCode = "tool_missing"
	IssueRuntimeConfigConflict  IssueCode = "runtime_config_conflict"
)

// DecodeIssueKind identifies a structural manifest decoding failure.
type DecodeIssueKind uint8

const (
	DecodeYAMLInvalid DecodeIssueKind = iota + 1
	DecodeSchemaInvalid
	DecodeDuplicateKey
	DecodeUnknownField
)

// DecodeIssue is a structured manifest persistence fact.
type DecodeIssue struct {
	Kind  DecodeIssueKind
	Field string
	// Line and Column are one-based; zero means the location is unavailable.
	Line   int
	Column int
}

// LoadManifestResult contains the selected project and structural manifest issues.
type LoadManifestResult struct {
	Project Project
	Issues  []DecodeIssue
}

// Issue is one stable, presentation-neutral project validation fact.
type Issue struct {
	Code IssueCode
	Path string
	// Line and Column are one-based; zero means the location is unavailable.
	Line       int
	Column     int
	Field      string
	Parameters *Parameters
}

// Parameters carries code-specific validation facts used by delivery renderers.
type Parameters struct {
	Expected string
	Actual   string
	Value    string
}
