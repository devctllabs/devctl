package project

import (
	"reflect"
	"sort"
	"strings"
	"unicode"
)

// RuntimeConfigType identifies the typed value loaded from one environment key.
type RuntimeConfigType string

const (
	RuntimeConfigString     RuntimeConfigType = "string"
	RuntimeConfigBool       RuntimeConfigType = "bool"
	RuntimeConfigInt        RuntimeConfigType = "int"
	RuntimeConfigDuration   RuntimeConfigType = "duration"
	RuntimeConfigStringList RuntimeConfigType = "string_list"
)

// RuntimeConfigScope selects one projection of the canonical runtime configuration.
type RuntimeConfigScope uint8

const (
	RuntimeConfigRuntime RuntimeConfigScope = 1 << iota
	RuntimeConfigExample
	RuntimeConfigInspect
)

// RuntimeConfigField is one effective environment-backed configuration fact.
type RuntimeConfigField struct {
	Group      string
	Name       string
	Key        string
	Type       RuntimeConfigType
	Default    any
	HasDefault bool
	Secret     bool
}

// RuntimeConfigCatalog is an immutable effective Runtime Config projection of one Manifest.
type RuntimeConfigCatalog struct {
	prefix  string
	entries []runtimeConfigEntry
}

type runtimeConfigEntry struct {
	field  RuntimeConfigField
	scopes RuntimeConfigScope
}

// RuntimeConfigConflictError reports declarations that cannot own the same effective key.
type RuntimeConfigConflictError struct {
	Key   string
	Field string
}

func (e *RuntimeConfigConflictError) Error() string {
	if e.Field != "" {
		return "conflicting runtime config field " + e.Field
	}
	return "conflicting runtime config declaration for " + e.Key
}

// NewRuntimeConfigCatalog resolves effective Runtime Config policy for manifest.
func NewRuntimeConfigCatalog(manifest Manifest) (RuntimeConfigCatalog, error) {
	builder := runtimeConfigBuilder{
		prefix: runtimeConfigPrefix(manifest),
		byKey:  make(map[string]*runtimeConfigCandidate),
	}
	builder.addDerived(manifest)
	if err := builder.addExplicit(manifest); err != nil {
		return RuntimeConfigCatalog{}, err
	}
	entries, err := builder.sortedEntries()
	if err != nil {
		return RuntimeConfigCatalog{}, err
	}
	return RuntimeConfigCatalog{prefix: builder.prefix, entries: entries}, nil
}

// Prefix returns the effective project environment prefix.
func (c RuntimeConfigCatalog) Prefix() string {
	return c.prefix
}

// Entries returns a defensive key-sorted projection for scope.
func (c RuntimeConfigCatalog) Entries(scope RuntimeConfigScope) []RuntimeConfigField {
	fields := make([]RuntimeConfigField, 0, len(c.entries))
	for _, entry := range c.entries {
		if entry.scopes&scope != 0 {
			fields = append(fields, entry.field)
		}
	}
	return fields
}

type runtimeConfigCandidate struct {
	entry    runtimeConfigEntry
	explicit *runtimeConfigDeclaration
}

type runtimeConfigDeclaration struct {
	group      string
	name       string
	typeName   RuntimeConfigType
	defaultVal any
	hasDefault bool
	secret     bool
}

type runtimeConfigBuilder struct {
	prefix string
	byKey  map[string]*runtimeConfigCandidate
}

type derivedRuntimeConfig struct {
	typeName   RuntimeConfigType
	defaultVal any
	hasDefault bool
	secret     bool
	scopes     RuntimeConfigScope
}

func runtimeDerived(typeName RuntimeConfigType, defaultValue any, hasDefault bool) derivedRuntimeConfig {
	return derivedRuntimeConfig{typeName: typeName, defaultVal: defaultValue, hasDefault: hasDefault, scopes: runtimeScopes()}
}

func runtimeDerivedSecret(typeName RuntimeConfigType, defaultValue any, hasDefault, secret bool) derivedRuntimeConfig {
	return derivedRuntimeConfig{typeName: typeName, defaultVal: defaultValue, hasDefault: hasDefault, secret: secret, scopes: runtimeScopes()}
}

func (b *runtimeConfigBuilder) addDerived(manifest Manifest) {
	b.addCapabilities(manifest)
	b.addDatabase(manifest.Components.DB)
	b.addRedis(manifest.Components.Redis)
	b.addS3(manifest.Components.S3)
	b.addClients(manifest.Components.HTTP, manifest.Components.GRPC)
	b.addMigrations(manifest.Components.DB)
}

func (b *runtimeConfigBuilder) addCapabilities(manifest Manifest) {
	components := manifest.Components
	if components.Logging != nil {
		b.derived("Logging", "Level", "LOG_LEVEL", runtimeDerived(RuntimeConfigString, "info", true))
	}
	if components.HTTP != nil && components.HTTP.Server != nil {
		b.derived("HTTP", "Address", "HTTP_ADDR", runtimeDerived(RuntimeConfigString, ":8080", true))
		b.start("HTTP", "Enabled", "HTTP_SERVER_ENABLED", components.HTTP.Server.Start)
	}
	if components.GRPC != nil && components.GRPC.Server != nil {
		b.derived("GRPC", "Address", "GRPC_ADDR", runtimeDerived(RuntimeConfigString, ":9090", true))
		b.start("GRPC", "Enabled", "GRPC_SERVER_ENABLED", components.GRPC.Server.Start)
	}
	if components.Kafka != nil {
		b.derived("Kafka", "Brokers", "KAFKA_BROKERS", runtimeDerived(RuntimeConfigStringList, "localhost:29092", true))
		for _, consumer := range components.Kafka.Consumers {
			name := runtimeConfigExportedName(consumer.Name)
			keyPart := runtimeConfigEnvName(consumer.Name)
			groupKey := valueOrDefault(consumer.GroupEnv, "KAFKA_"+keyPart+"_GROUP")
			b.derived("Kafka", name+"Group", groupKey, runtimeDerived(RuntimeConfigString, manifest.Project.Name+"-"+consumer.Name+"-group", true))
			b.derived("Kafka", name+"Topic", "KAFKA_"+keyPart+"_TOPIC", runtimeDerived(RuntimeConfigString, consumer.Topic, true))
			b.derived("Kafka", name+"BatchMaxSize", "KAFKA_"+keyPart+"_BATCH_MAX_SIZE", runtimeDerived(RuntimeConfigInt, 1, true))
			b.derived("Kafka", name+"BatchFlushInterval", "KAFKA_"+keyPart+"_BATCH_FLUSH_INTERVAL", runtimeDerived(RuntimeConfigDuration, "1s", true))
			b.derived("Kafka", name+"RetryMaxAttempts", "KAFKA_"+keyPart+"_RETRY_MAX_ATTEMPTS", runtimeDerived(RuntimeConfigInt, 3, true))
			b.derived("Kafka", name+"RetryMaxElapsedTime", "KAFKA_"+keyPart+"_RETRY_MAX_ELAPSED_TIME", runtimeDerived(RuntimeConfigDuration, "0s", true))
			b.derived("Kafka", name+"RetryInitialDelay", "KAFKA_"+keyPart+"_RETRY_INITIAL_DELAY", runtimeDerived(RuntimeConfigDuration, "1s", true))
			b.derived("Kafka", name+"RetryMaxDelay", "KAFKA_"+keyPart+"_RETRY_MAX_DELAY", runtimeDerived(RuntimeConfigDuration, "30s", true))
			b.derived("Kafka", name+"RebalanceTimeout", "KAFKA_"+keyPart+"_REBALANCE_TIMEOUT", runtimeDerived(RuntimeConfigDuration, "30s", true))
			b.derived("Kafka", name+"RebalanceDrainTimeout", "KAFKA_"+keyPart+"_REBALANCE_DRAIN_TIMEOUT", runtimeDerived(RuntimeConfigDuration, "20s", true))
			b.derived("Kafka", name+"ShutdownTimeout", "KAFKA_"+keyPart+"_SHUTDOWN_TIMEOUT", runtimeDerived(RuntimeConfigDuration, "30s", true))
			b.start("Kafka", name+"Enabled", "KAFKA_"+keyPart+"_CONSUMER_ENABLED", consumer.Start)
		}
		for _, producer := range components.Kafka.Producers {
			name := runtimeConfigExportedName(producer.Name)
			key := valueOrDefault(producer.TopicEnv, "KAFKA_"+runtimeConfigEnvName(producer.Name)+"_TOPIC")
			b.derived("Kafka", name+"Topic", key, runtimeDerived(RuntimeConfigString, producer.Topic, producer.Topic != ""))
		}
	}
	if components.Health != nil {
		b.derived("Health", "Address", "HEALTH_ADDR", runtimeDerived(RuntimeConfigString, ":8081", true))
		if components.Health.Server != nil {
			b.start("Health", "Enabled", "HEALTH_SERVER_ENABLED", components.Health.Server.Start)
		}
	}
	if components.Telemetry != nil {
		b.start("Telemetry", "Enabled", "TELEMETRY_ENABLED", components.Telemetry.Start)
		b.derived("Telemetry", "ServiceVersion", "SERVICE_VERSION", runtimeDerived(RuntimeConfigString, "dev", true))
		b.derived("Telemetry", "DeploymentEnvironment", "DEPLOYMENT_ENVIRONMENT", runtimeDerived(RuntimeConfigString, "development", true))
	}
	if pprof := manifest.Languages.Go.Components.Pprof; pprof != nil {
		b.derived("Pprof", "Address", "PPROF_ADDR", runtimeDerived(RuntimeConfigString, "127.0.0.1:6060", true))
		if pprof.Server != nil {
			b.start("Pprof", "Enabled", "PPROF_ENABLED", pprof.Server.Start)
		}
	}
}

func (b *runtimeConfigBuilder) start(group, name, fallbackKey string, start *Start) {
	if start == nil {
		return
	}
	defaultValue := false
	if start.Default != nil {
		defaultValue = *start.Default
	}
	b.derived(group, name, valueOrDefault(start.Env, fallbackKey), runtimeDerived(RuntimeConfigBool, defaultValue, true))
}

func (b *runtimeConfigBuilder) addDatabase(database *DB) {
	if database == nil {
		return
	}
	for _, connection := range database.Connections {
		connectionName := runtimeConfigExportedName(connection.Name)
		group := "DB" + connectionName
		defaultVariant := connection.Default
		if defaultVariant == "" && len(connection.Variants) == 1 {
			defaultVariant = connection.Variants[0].Name
		}
		kindKey := valueOrDefault(connection.KindEnv, "DB_"+runtimeConfigEnvName(connection.Name)+"_KIND")
		b.derived(group, "Kind", kindKey, runtimeDerived(RuntimeConfigString, defaultVariant, defaultVariant != ""))
		for _, variant := range connection.Variants {
			key := valueOrDefault(variant.DSNEnv, "DB_"+runtimeConfigEnvName(connection.Name)+"_"+runtimeConfigEnvName(variant.Name)+"_DSN")
			b.derived(group, runtimeConfigExportedName(variant.Name)+"DSN", key, runtimeDerivedSecret(RuntimeConfigString, variant.DSNDefault, variant.DSNDefault != "", variant.Secret))
		}
	}
}

func (b *runtimeConfigBuilder) addRedis(redis *Redis) {
	if redis == nil {
		return
	}
	for _, connection := range redis.Connections {
		key := valueOrDefault(connection.AddrEnv, "REDIS_"+runtimeConfigEnvName(connection.Name)+"_ADDR")
		b.derived("Redis", runtimeConfigExportedName(connection.Name)+"Address", key, runtimeDerived(RuntimeConfigString, connection.AddrDefault, connection.AddrDefault != ""))
	}
}

func (b *runtimeConfigBuilder) addS3(storage *S3) {
	if storage == nil {
		return
	}
	for _, connection := range storage.Connections {
		keyPrefix := "S3"
		fieldPrefix := ""
		if connection.Name != "" && connection.Name != "default" {
			keyPrefix += "_" + runtimeConfigEnvName(connection.Name)
			fieldPrefix = runtimeConfigExportedName(connection.Name)
		}
		b.derived("S3", fieldPrefix+"Endpoint", keyPrefix+"_ENDPOINT", runtimeDerived(RuntimeConfigString, connection.Endpoint, connection.Endpoint != ""))
		b.derived("S3", fieldPrefix+"Region", keyPrefix+"_REGION", runtimeDerived(RuntimeConfigString, connection.Region, connection.Region != ""))
		b.derived("S3", fieldPrefix+"ForcePathStyle", keyPrefix+"_FORCE_PATH_STYLE", runtimeDerived(RuntimeConfigBool, connection.PathStyle, true))
		if connection.Credentials == "static" {
			accessKey := valueOrDefault(connection.AccessKeyEnv, keyPrefix+"_ACCESS_KEY_ID")
			secretKey := valueOrDefault(connection.SecretKeyEnv, keyPrefix+"_SECRET_ACCESS_KEY")
			b.derived("S3", fieldPrefix+"AccessKeyID", accessKey, runtimeDerivedSecret(RuntimeConfigString, nil, false, true))
			b.derived("S3", fieldPrefix+"SecretAccessKey", secretKey, runtimeDerivedSecret(RuntimeConfigString, nil, false, true))
		}
	}
	for _, bucket := range storage.Buckets {
		key := "S3_" + runtimeConfigEnvName(bucket.Name) + "_BUCKET"
		b.derived("S3", runtimeConfigExportedName(bucket.Name)+"Bucket", key, runtimeDerived(RuntimeConfigString, bucket.Bucket, bucket.Bucket != ""))
	}
}

func (b *runtimeConfigBuilder) addClients(http *HTTP, grpc *GRPC) {
	if http != nil {
		for _, client := range http.Clients {
			key := valueOrDefault(client.BaseURLEnv, "HTTP_"+runtimeConfigEnvName(client.Name)+"_BASE_URL")
			b.derived("HTTPClients", runtimeConfigExportedName(client.Name)+"BaseURL", key, runtimeDerived(RuntimeConfigString, nil, false))
		}
	}
	if grpc != nil {
		for _, client := range grpc.Clients {
			key := valueOrDefault(client.AddrEnv, "GRPC_"+runtimeConfigEnvName(client.Name)+"_ADDR")
			b.derived("GRPCClients", runtimeConfigExportedName(client.Name)+"Address", key, runtimeDerived(RuntimeConfigString, nil, false))
		}
	}
}

func (b *runtimeConfigBuilder) addMigrations(database *DB) {
	if database == nil {
		return
	}
	for _, connection := range database.Connections {
		for _, variant := range connection.Variants {
			if variant.Migrations == nil || variant.Migrations.DatabaseEnv == "" {
				continue
			}
			name := "DB" + runtimeConfigExportedName(connection.Name) + runtimeConfigExportedName(variant.Name) + "MigrationsURL"
			b.derived("Migrations", name, variant.Migrations.DatabaseEnv, derivedRuntimeConfig{
				typeName: RuntimeConfigString, defaultVal: variant.Migrations.DatabaseDefault,
				hasDefault: variant.Migrations.DatabaseDefault != "", secret: variant.Kind == "postgres" || variant.Kind == "clickhouse",
				scopes: RuntimeConfigExample | RuntimeConfigInspect,
			})
		}
	}
}

func (b *runtimeConfigBuilder) addExplicit(manifest Manifest) error {
	for _, group := range manifest.Env.Custom {
		if err := b.explicitVars(runtimeConfigExportedName(group.Group), group.Vars); err != nil {
			return err
		}
	}
	for _, environment := range explicitComponentEnvironments(manifest) {
		if err := b.explicitEnv(environment.group, environment.environment); err != nil {
			return err
		}
	}
	return nil
}

type namedComponentEnvironment struct {
	group       string
	environment ComponentEnv
}

func explicitComponentEnvironments(manifest Manifest) []namedComponentEnvironment {
	components := manifest.Components
	result := make([]namedComponentEnvironment, 0, 10)
	if components.HTTP != nil {
		result = append(result, namedComponentEnvironment{"HTTP", components.HTTP.Env})
	}
	if components.GRPC != nil {
		result = append(result, namedComponentEnvironment{"GRPC", components.GRPC.Env})
	}
	if components.Kafka != nil {
		result = append(result, namedComponentEnvironment{"Kafka", components.Kafka.Env})
	}
	if components.Logging != nil {
		result = append(result, namedComponentEnvironment{"Logging", components.Logging.Env})
	}
	if components.Health != nil {
		result = append(result, namedComponentEnvironment{"Health", components.Health.Env})
	}
	if components.Telemetry != nil {
		result = append(result, namedComponentEnvironment{"Telemetry", components.Telemetry.Env})
	}
	if components.DB != nil {
		result = append(result, namedComponentEnvironment{"Database", components.DB.Env})
	}
	if components.Redis != nil {
		result = append(result, namedComponentEnvironment{"Redis", components.Redis.Env})
	}
	if components.S3 != nil {
		result = append(result, namedComponentEnvironment{"S3", components.S3.Env})
	}
	if pprof := manifest.Languages.Go.Components.Pprof; pprof != nil {
		result = append(result, namedComponentEnvironment{"Pprof", pprof.Env})
	}
	return result
}

func (b *runtimeConfigBuilder) explicitEnv(group string, environment ComponentEnv) error {
	variables := append(append([]EnvVar(nil), environment.System...), environment.Custom...)
	return b.explicitVars(group, variables)
}

func (b *runtimeConfigBuilder) explicitVars(group string, variables []EnvVar) error {
	for _, variable := range variables {
		if variable.Key == "" {
			continue
		}
		name := runtimeConfigCustomName(group, variable.Key)
		declaration := runtimeConfigDeclaration{
			group: group, name: name, typeName: runtimeConfigType(variable.Type),
			defaultVal: variable.Default, hasDefault: variable.Default != nil, secret: variable.Secret,
		}
		if declaration.secret {
			declaration.defaultVal = nil
			declaration.hasDefault = false
		}
		key := b.key(variable.Key)
		existing := b.byKey[key]
		if existing == nil {
			b.byKey[key] = &runtimeConfigCandidate{
				entry: runtimeConfigEntry{field: RuntimeConfigField{
					Group: group, Name: name, Key: key, Type: declaration.typeName,
					Default: declaration.defaultVal, HasDefault: declaration.hasDefault, Secret: declaration.secret,
				}, scopes: runtimeScopes()},
				explicit: &declaration,
			}
			continue
		}
		if existing.explicit != nil && !reflect.DeepEqual(*existing.explicit, declaration) {
			return &RuntimeConfigConflictError{Key: key}
		}
		if existing.explicit == nil {
			existing.explicit = &declaration
			existing.entry.field.Type = declaration.typeName
			existing.entry.field.Default = declaration.defaultVal
			existing.entry.field.HasDefault = declaration.hasDefault
			existing.entry.field.Secret = declaration.secret
		}
	}
	return nil
}

func (b *runtimeConfigBuilder) derived(group, name, key string, config derivedRuntimeConfig) {
	if key == "" {
		return
	}
	if config.secret {
		config.defaultVal = nil
		config.hasDefault = false
	}
	finalKey := b.key(key)
	if _, exists := b.byKey[finalKey]; exists {
		return
	}
	b.byKey[finalKey] = &runtimeConfigCandidate{entry: runtimeConfigEntry{field: RuntimeConfigField{
		Group: group, Name: name, Key: finalKey, Type: config.typeName,
		Default: config.defaultVal, HasDefault: config.hasDefault, Secret: config.secret,
	}, scopes: config.scopes}}
}

func (b *runtimeConfigBuilder) key(key string) string {
	if strings.HasPrefix(key, "OTEL_") {
		return key
	}
	return b.prefix + key
}

func (b *runtimeConfigBuilder) sortedEntries() ([]runtimeConfigEntry, error) {
	entries := make([]runtimeConfigEntry, 0, len(b.byKey))
	paths := make(map[string]string, len(b.byKey))
	for key, candidate := range b.byKey {
		fieldPath := candidate.entry.field.Group + "." + candidate.entry.field.Name
		if otherKey, exists := paths[fieldPath]; exists && otherKey != key {
			return nil, &RuntimeConfigConflictError{Field: fieldPath}
		}
		paths[fieldPath] = key
		entries = append(entries, candidate.entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].field.Key < entries[j].field.Key })
	return entries, nil
}

func runtimeConfigPrefix(manifest Manifest) string {
	if manifest.Env.Prefix != "" {
		return manifest.Env.Prefix
	}
	return runtimeConfigEnvName(manifest.Project.Name) + "_"
}

func runtimeConfigType(value string) RuntimeConfigType {
	switch value {
	case "bool":
		return RuntimeConfigBool
	case "int":
		return RuntimeConfigInt
	case "duration":
		return RuntimeConfigDuration
	default:
		return RuntimeConfigString
	}
}

func runtimeConfigCustomName(group, key string) string {
	if group == "Telemetry" && strings.HasPrefix(key, "OTEL_") {
		key = strings.TrimPrefix(key, "OTEL_")
	}
	return runtimeConfigExportedName(key)
}

func runtimeConfigEnvName(value string) string {
	return strings.ToUpper(strings.ReplaceAll(value, "-", "_"))
}

func runtimeConfigExportedName(value string) string {
	parts := strings.FieldsFunc(value, func(char rune) bool {
		return char == '_' || char == '-' || !unicode.IsLetter(char) && !unicode.IsDigit(char)
	})
	var builder strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = unicode.ToUpper(runes[0])
		builder.WriteString(string(runes))
	}
	if builder.Len() == 0 {
		return "Value"
	}
	name := builder.String()
	if unicode.IsDigit([]rune(name)[0]) {
		return "Value" + name
	}
	return name
}

func runtimeScopes() RuntimeConfigScope {
	return RuntimeConfigRuntime | RuntimeConfigExample | RuntimeConfigInspect
}
