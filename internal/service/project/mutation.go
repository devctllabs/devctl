package project

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

var kebabCase = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var environmentKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

type mutation func(manifest *projectdomain.Manifest) error

// Enable applies one capability mutation to a structurally and semantically valid manifest.
func (s *Service) Enable(ctx context.Context, command projectdomain.EnableCommand) (projectdomain.ManifestResult, error) {
	return s.mutate(ctx, command.ManifestPath, func(manifest *projectdomain.Manifest) error {
		return enableCapability(manifest, command)
	})
}

// AddDB adds one database connection using the mutation command's conflict policy.
func (s *Service) AddDB(ctx context.Context, command projectdomain.AddDBCommand) (projectdomain.ManifestResult, error) {
	return s.mutate(ctx, command.ManifestPath, func(manifest *projectdomain.Manifest) error {
		return addDatabase(manifest, command)
	})
}

// AddSource adds one contract source using the mutation command's conflict policy.
func (s *Service) AddSource(ctx context.Context, command projectdomain.AddSourceCommand) (projectdomain.ManifestResult, error) {
	return s.mutate(ctx, command.ManifestPath, func(manifest *projectdomain.Manifest) error {
		return addSource(manifest, command)
	})
}

// AddHTTPClient adds one generated HTTP client using the mutation command's conflict policy.
func (s *Service) AddHTTPClient(ctx context.Context, command projectdomain.AddHTTPClientCommand) (projectdomain.ManifestResult, error) {
	return s.mutate(ctx, command.ManifestPath, func(manifest *projectdomain.Manifest) error {
		return addHTTPClient(manifest, command)
	})
}

func (s *Service) AddGRPCClient(ctx context.Context, command projectdomain.AddGRPCClientCommand) (projectdomain.ManifestResult, error) {
	return s.mutate(ctx, command.ManifestPath, func(manifest *projectdomain.Manifest) error { return addGRPCClient(manifest, command) })
}

func (s *Service) AddKafkaConsumer(ctx context.Context, command projectdomain.AddKafkaConsumerCommand) (projectdomain.ManifestResult, error) {
	return s.mutate(ctx, command.ManifestPath, func(manifest *projectdomain.Manifest) error { return addKafkaConsumer(manifest, command) })
}

func (s *Service) AddKafkaProducer(ctx context.Context, command projectdomain.AddKafkaProducerCommand) (projectdomain.ManifestResult, error) {
	return s.mutate(ctx, command.ManifestPath, func(manifest *projectdomain.Manifest) error { return addKafkaProducer(manifest, command) })
}

func (s *Service) AddRedis(ctx context.Context, command projectdomain.AddRedisCommand) (projectdomain.ManifestResult, error) {
	return s.mutate(ctx, command.ManifestPath, func(manifest *projectdomain.Manifest) error { return addRedis(manifest, command) })
}

func (s *Service) AddS3Connection(ctx context.Context, command projectdomain.AddS3ConnectionCommand) (projectdomain.ManifestResult, error) {
	return s.mutate(ctx, command.ManifestPath, func(manifest *projectdomain.Manifest) error { return addS3Connection(manifest, command) })
}

func (s *Service) AddS3(ctx context.Context, command projectdomain.AddS3Command) (projectdomain.ManifestResult, error) {
	return s.mutate(ctx, command.ManifestPath, func(manifest *projectdomain.Manifest) error { return addS3(manifest, command) })
}

func (s *Service) mutate(ctx context.Context, manifestPath string, apply mutation) (projectdomain.ManifestResult, error) {
	selected, err := s.loadValidProject(ctx, manifestPath)
	if err != nil {
		return projectdomain.ManifestResult{}, fmt.Errorf("s.loadValidProject: %w", err)
	}
	before := cloneManifest(selected.Manifest)
	if err := apply(&selected.Manifest); err != nil {
		return projectdomain.ManifestResult{Manifest: selected.ManifestPath}, err
	}
	if reflect.DeepEqual(before, selected.Manifest) {
		return projectdomain.ManifestResult{Manifest: selected.ManifestPath, Change: projectdomain.ChangeUnchanged}, nil
	}
	if _, err := s.manifests.Save(ctx, selected); err != nil {
		operationErr := projectOperationError(projectdomain.OperationSaveManifest, selected.ManifestPath, projectdomain.FailureUnavailable, err)
		return projectdomain.ManifestResult{Manifest: selected.ManifestPath}, fmt.Errorf("manifests.Save: %w", operationErr)
	}
	return projectdomain.ManifestResult{Manifest: selected.ManifestPath, Change: projectdomain.ChangeUpdated}, nil
}

func cloneManifest(source projectdomain.Manifest) projectdomain.Manifest {
	clone := source
	clone.Sources = cloneMap(source.Sources)
	clone.Exports = cloneMap(source.Exports)
	clone.Env.Custom = append([]projectdomain.EnvGroup(nil), source.Env.Custom...)
	clone.Components = cloneComponents(source.Components)
	clone.Languages = cloneLanguages(source.Languages)
	return clone
}

func cloneMap[K comparable, V any](source map[K]V) map[K]V {
	if source == nil {
		return nil
	}
	clone := make(map[K]V, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneComponents(source projectdomain.Components) projectdomain.Components {
	clone := source
	clone.HTTP = cloneHTTP(source.HTTP)
	clone.GRPC = cloneGRPC(source.GRPC)
	clone.Kafka = cloneKafka(source.Kafka)
	if source.Logging != nil {
		value := *source.Logging
		clone.Logging = &value
	}
	clone.Health = cloneHealth(source.Health)
	if source.Telemetry != nil {
		value := *source.Telemetry
		value.Start = cloneStart(source.Telemetry.Start)
		clone.Telemetry = &value
	}
	clone.DB = cloneDB(source.DB)
	if source.Redis != nil {
		redis := *source.Redis
		redis.Connections = append([]projectdomain.RedisConnection(nil), source.Redis.Connections...)
		clone.Redis = &redis
	}
	clone.S3 = cloneS3(source.S3)
	return clone
}

func cloneHTTP(source *projectdomain.HTTP) *projectdomain.HTTP {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Clients = append([]projectdomain.HTTPClient(nil), source.Clients...)
	if source.Server != nil {
		server := *source.Server
		server.Start = cloneStart(source.Server.Start)
		clone.Server = &server
	}
	return &clone
}

func cloneGRPC(source *projectdomain.GRPC) *projectdomain.GRPC {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Clients = append([]projectdomain.GRPCClient(nil), source.Clients...)
	if source.Server != nil {
		server := *source.Server
		server.Start = cloneStart(source.Server.Start)
		clone.Server = &server
	}
	return &clone
}

func cloneKafka(source *projectdomain.Kafka) *projectdomain.Kafka {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Consumers = append([]projectdomain.KafkaConsumer(nil), source.Consumers...)
	for index := range clone.Consumers {
		clone.Consumers[index].Start = cloneStart(clone.Consumers[index].Start)
	}
	clone.Producers = append([]projectdomain.KafkaProducer(nil), source.Producers...)
	return &clone
}

func cloneHealth(source *projectdomain.Health) *projectdomain.Health {
	if source == nil {
		return nil
	}
	clone := *source
	if source.Server != nil {
		server := *source.Server
		server.Start = cloneStart(source.Server.Start)
		clone.Server = &server
	}
	return &clone
}

func cloneDB(source *projectdomain.DB) *projectdomain.DB {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Connections = append([]projectdomain.DBConnection(nil), source.Connections...)
	for index := range clone.Connections {
		clone.Connections[index].Variants = append([]projectdomain.DBVariant(nil), source.Connections[index].Variants...)
		for variantIndex := range clone.Connections[index].Variants {
			migrations := clone.Connections[index].Variants[variantIndex].Migrations
			if migrations != nil {
				migrationClone := *migrations
				clone.Connections[index].Variants[variantIndex].Migrations = &migrationClone
			}
		}
	}
	return &clone
}

func cloneS3(source *projectdomain.S3) *projectdomain.S3 {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Connections = append([]projectdomain.S3Connection(nil), source.Connections...)
	clone.Buckets = append([]projectdomain.S3Bucket(nil), source.Buckets...)
	return &clone
}

func cloneLanguages(source projectdomain.Languages) projectdomain.Languages {
	clone := source
	if source.Go.Components.Pprof != nil {
		pprof := *source.Go.Components.Pprof
		if source.Go.Components.Pprof.Server != nil {
			server := *source.Go.Components.Pprof.Server
			server.Start = cloneStart(source.Go.Components.Pprof.Server.Start)
			pprof.Server = &server
		}
		clone.Go.Components.Pprof = &pprof
	}
	return clone
}

func cloneStart(source *projectdomain.Start) *projectdomain.Start {
	if source == nil {
		return nil
	}
	clone := *source
	if source.Default != nil {
		value := *source.Default
		clone.Default = &value
	}
	return &clone
}

func enableCapability(manifest *projectdomain.Manifest, command projectdomain.EnableCommand) error {
	switch command.Capability {
	case "http":
		return enableHTTP(manifest, command)
	case "grpc":
		return enableGRPC(manifest, command)
	case "logging":
		if command.Always {
			return invalidMutation(projectdomain.MutationUnsupportedOption, "always", "true")
		}
		if manifest.Components.Logging == nil {
			manifest.Components.Logging = &projectdomain.Logging{}
		}
		return nil
	case "health":
		return enableHealth(manifest, command)
	case "telemetry":
		return enableTelemetry(manifest, command)
	case "pprof":
		return enablePprof(manifest, command)
	default:
		return invalidMutation(projectdomain.MutationUnsupportedValue, "capability", command.Capability)
	}
}

func enableGRPC(manifest *projectdomain.Manifest, command projectdomain.EnableCommand) error {
	desired := desiredStart(command.Always, "GRPC_SERVER_ENABLED", true)
	if manifest.Components.GRPC == nil {
		manifest.Components.GRPC = &projectdomain.GRPC{}
	}
	if manifest.Components.GRPC.Server == nil {
		manifest.Components.GRPC.Server = &projectdomain.GRPCServer{
			ProtoRoot: "api/proto/grpc",
			BufConfig: "buf.yaml",
			Start:     desired,
		}
	}
	if !reflect.DeepEqual(manifest.Components.GRPC.Server.Start, desired) && !command.Force {
		return mutationConflict("components.grpc.server.start", "")
	}
	manifest.Components.GRPC.Server.Start = desired
	if manifest.Languages.Go.Generators.GRPC == nil {
		manifest.Languages.Go.Generators.GRPC = &projectdomain.GRPCGenerator{Out: "gen/grpc", BufGenConfig: "tools/buf/grpc.gen.yaml"}
	}
	return nil
}

func desiredStart(always bool, environment string, defaultValue bool) *projectdomain.Start {
	if always {
		return nil
	}
	return &projectdomain.Start{Env: environment, Default: &defaultValue}
}

func enableHTTP(manifest *projectdomain.Manifest, command projectdomain.EnableCommand) error {
	desired := desiredStart(command.Always, "HTTP_SERVER_ENABLED", true)
	if manifest.Components.HTTP == nil {
		manifest.Components.HTTP = &projectdomain.HTTP{}
	}
	if manifest.Components.HTTP.Server == nil {
		manifest.Components.HTTP.Server = &projectdomain.HTTPServer{OpenAPI: "api/openapi/swagger.yaml", Start: desired}
	}
	if !reflect.DeepEqual(manifest.Components.HTTP.Server.Start, desired) && !command.Force {
		return mutationConflict("components.http.server.start", "")
	}
	manifest.Components.HTTP.Server.Start = desired
	return nil
}

func enableHealth(manifest *projectdomain.Manifest, command projectdomain.EnableCommand) error {
	desired := desiredStart(command.Always, "HEALTH_SERVER_ENABLED", true)
	if manifest.Components.Health == nil {
		manifest.Components.Health = &projectdomain.Health{}
	}
	if manifest.Components.Health.Server == nil {
		manifest.Components.Health.Server = &projectdomain.HealthServer{Start: desired}
	}
	if !reflect.DeepEqual(manifest.Components.Health.Server.Start, desired) && !command.Force {
		return mutationConflict("components.health.server.start", "")
	}
	manifest.Components.Health.Server.Start = desired
	return nil
}

func enableTelemetry(manifest *projectdomain.Manifest, command projectdomain.EnableCommand) error {
	desired := desiredStart(command.Always, "TELEMETRY_ENABLED", false)
	if manifest.Components.Telemetry == nil {
		manifest.Components.Telemetry = &projectdomain.Telemetry{Start: desired}
	}
	if !reflect.DeepEqual(manifest.Components.Telemetry.Start, desired) && !command.Force {
		return mutationConflict("components.telemetry.start", "")
	}
	manifest.Components.Telemetry.Start = desired
	return nil
}

func enablePprof(manifest *projectdomain.Manifest, command projectdomain.EnableCommand) error {
	desired := desiredStart(command.Always, "PPROF_ENABLED", false)
	if manifest.Languages.Go.Components.Pprof == nil {
		manifest.Languages.Go.Components.Pprof = &projectdomain.Pprof{}
	}
	if manifest.Languages.Go.Components.Pprof.Server == nil {
		manifest.Languages.Go.Components.Pprof.Server = &projectdomain.PprofServer{Start: desired}
	}
	if !reflect.DeepEqual(manifest.Languages.Go.Components.Pprof.Server.Start, desired) && !command.Force {
		return mutationConflict("languages.go.components.pprof.server.start", "")
	}
	manifest.Languages.Go.Components.Pprof.Server.Start = desired
	return nil
}

func addSource(manifest *projectdomain.Manifest, command projectdomain.AddSourceCommand) error {
	if !kebabCase.MatchString(command.Name) {
		return invalidMutation(projectdomain.MutationInvalidName, "source.name", command.Name)
	}
	desired := projectdomain.Source{Type: projectdomain.SourceType(command.Type), Path: command.Path, URL: command.URL, Filename: command.Filename, AllowInsecureHTTP: command.AllowInsecureHTTP, Repo: command.Repo, Ref: command.Ref, Proto: projectdomain.SourceProto{BufConfig: command.BufConfig}}
	if err := validateSource(desired); err != nil {
		return err
	}
	if current, exists := manifest.Sources[command.Name]; exists && !reflect.DeepEqual(current, desired) && !command.Force {
		return mutationConflict("sources."+command.Name, command.Name)
	}
	if manifest.Sources == nil {
		manifest.Sources = make(map[string]projectdomain.Source)
	}
	manifest.Sources[command.Name] = desired
	return nil
}

func validateSource(source projectdomain.Source) error {
	if source.Proto.BufConfig != "" && !safeRelative(source.Proto.BufConfig) {
		return invalidMutation(projectdomain.MutationInvalidOptions, "source.proto.buf_config", source.Proto.BufConfig)
	}
	switch source.Type {
	case projectdomain.SourceLocal:
		if !safeRelative(source.Path) || source.URL != "" || source.Repo != "" || source.Ref != "" || source.Filename != "" || source.AllowInsecureHTTP {
			return invalidMutation(projectdomain.MutationInvalidOptions, "source", string(source.Type))
		}
	case projectdomain.SourceURL:
		return validateURLSource(source)
	case projectdomain.SourceGit:
		if source.Repo == "" || source.Ref == "" || (source.Path != "" && !safeRelative(source.Path)) || source.URL != "" || source.Filename != "" || source.AllowInsecureHTTP {
			return invalidMutation(projectdomain.MutationInvalidOptions, "source", string(source.Type))
		}
	case projectdomain.SourceDevctl:
		if source.Repo == "" || source.Ref == "" || source.Path != "" || source.URL != "" || source.Filename != "" || source.AllowInsecureHTTP {
			return invalidMutation(projectdomain.MutationInvalidOptions, "source", string(source.Type))
		}
	default:
		return invalidMutation(projectdomain.MutationUnsupportedValue, "source.type", string(source.Type))
	}
	return nil
}

func validateURLSource(source projectdomain.Source) error {
	parsed, err := url.Parse(source.URL)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return invalidMutation(projectdomain.MutationInvalidURL, "source.url", source.URL)
	}
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !source.AllowInsecureHTTP) {
		return invalidMutation(projectdomain.MutationInsecureURL, "source.url", source.URL)
	}
	if source.Path != "" || source.Repo != "" || source.Ref != "" {
		return invalidMutation(projectdomain.MutationInvalidOptions, "source", string(source.Type))
	}
	if source.Filename != "" && strings.Contains(source.Filename, "/") {
		return invalidMutation(projectdomain.MutationInvalidOptions, "source.filename", source.Filename)
	}
	return nil
}

func addDatabase(manifest *projectdomain.Manifest, command projectdomain.AddDBCommand) error {
	if !kebabCase.MatchString(command.Name) {
		return invalidMutation(projectdomain.MutationInvalidName, "database.name", command.Name)
	}
	if command.Kind != "sqlite" && command.Kind != "postgres" && command.Kind != "clickhouse" {
		return invalidMutation(projectdomain.MutationUnsupportedValue, "database.kind", command.Kind)
	}
	if !validMigrationOptions(command) {
		return invalidMutation(projectdomain.MutationInvalidOptions, "database.migrations", command.MigrationsPath)
	}
	if manifest.Components.DB == nil {
		manifest.Components.DB = &projectdomain.DB{}
	}
	connection := findConnection(manifest.Components.DB, command.Name)
	variant := defaultDBVariant(command)
	if connection == nil {
		manifest.Components.DB.Connections = append(manifest.Components.DB.Connections, projectdomain.DBConnection{Name: command.Name, Default: command.Kind, Variants: []projectdomain.DBVariant{variant}})
		return nil
	}
	addingClickHouseToExisting := command.Kind == "clickhouse" && len(connection.Variants) > 0
	addingTransactionalToClickHouse := command.Kind != "clickhouse" && connectionHasKind(connection, "clickhouse")
	if addingClickHouseToExisting || addingTransactionalToClickHouse {
		return invalidMutation(projectdomain.MutationInvalidOptions, "database.kind", command.Kind)
	}
	if err := upsertVariant(connection, variant, command.Force); err != nil {
		return err
	}
	if command.Default || connection.Default == "" {
		connection.Default = command.Kind
	}
	return nil
}

func validMigrationOptions(command projectdomain.AddDBCommand) bool {
	if command.NoMigrations && command.MigrationsPath != "" {
		return false
	}
	return command.MigrationsPath == "" || safeRelative(command.MigrationsPath)
}

func connectionHasKind(connection *projectdomain.DBConnection, kind string) bool {
	for _, variant := range connection.Variants {
		if variant.Kind == kind {
			return true
		}
	}
	return false
}

func findConnection(database *projectdomain.DB, name string) *projectdomain.DBConnection {
	for index := range database.Connections {
		if database.Connections[index].Name == name {
			return &database.Connections[index]
		}
	}
	return nil
}

func upsertVariant(connection *projectdomain.DBConnection, desired projectdomain.DBVariant, force bool) error {
	for index, current := range connection.Variants {
		if current.Name != desired.Name {
			continue
		}
		if !reflect.DeepEqual(current, desired) && !force {
			return mutationConflict("database.variant", desired.Name)
		}
		connection.Variants[index] = desired
		return nil
	}
	connection.Variants = append(connection.Variants, desired)
	return nil
}

func defaultDBVariant(command projectdomain.AddDBCommand) projectdomain.DBVariant {
	upper := strings.ToUpper(strings.ReplaceAll(command.Name, "-", "_"))
	kindUpper := strings.ToUpper(command.Kind)
	variant := projectdomain.DBVariant{Name: command.Kind, Kind: command.Kind, DSNEnv: "DB_" + upper + "_" + kindUpper + "_DSN", Secret: command.Kind == "postgres" || command.Kind == "clickhouse"}
	if command.Kind == "sqlite" {
		variant.DSNDefault = "file:./data/" + command.Name + ".db?_foreign_keys=on"
	}
	if command.Kind == "clickhouse" {
		variant.DSNDefault = "clickhouse://localhost:9000/default"
	}
	if !command.NoMigrations {
		path := command.MigrationsPath
		if path == "" {
			path = "migrations/" + command.Name + "/" + command.Kind
		}
		variant.Migrations = &projectdomain.DBMigrations{Path: path, DatabaseEnv: "DB_" + upper + "_" + kindUpper + "_MIGRATIONS_URL"}
		if command.Kind == "sqlite" {
			variant.Migrations.DatabaseDefault = "sqlite://./data/" + command.Name + ".db?_pragma=foreign_keys%281%29"
		}
	}
	return variant
}

func addHTTPClient(manifest *projectdomain.Manifest, command projectdomain.AddHTTPClientCommand) error {
	if !kebabCase.MatchString(command.Name) {
		return invalidMutation(projectdomain.MutationInvalidName, "http_client.name", command.Name)
	}
	source, exists := manifest.Sources[command.Source]
	if !exists {
		return invalidMutation(projectdomain.MutationNotFound, "http_client.source", command.Source)
	}
	if source.Type == projectdomain.SourceDevctl && (command.Export == "" || command.Path != "") {
		return invalidMutation(projectdomain.MutationInvalidOptions, "http_client", command.Name)
	}
	if source.Type != projectdomain.SourceDevctl && (command.Path == "" || command.Export != "") {
		return invalidMutation(projectdomain.MutationInvalidOptions, "http_client", command.Name)
	}
	if manifest.Components.HTTP == nil {
		manifest.Components.HTTP = &projectdomain.HTTP{}
	}
	desired := projectdomain.HTTPClient{Name: command.Name, Source: command.Source, Export: command.Export, Path: command.Path, BaseURLEnv: command.BaseURLEnv}
	for index, current := range manifest.Components.HTTP.Clients {
		if current.Name != desired.Name {
			continue
		}
		if !reflect.DeepEqual(current, desired) && !command.Force {
			return mutationConflict("http_client", desired.Name)
		}
		manifest.Components.HTTP.Clients[index] = desired
		return nil
	}
	manifest.Components.HTTP.Clients = append(manifest.Components.HTTP.Clients, desired)
	return nil
}

func addGRPCClient(manifest *projectdomain.Manifest, command projectdomain.AddGRPCClientCommand) error {
	if !kebabCase.MatchString(command.Name) {
		return invalidMutation(projectdomain.MutationInvalidName, "grpc_client.name", command.Name)
	}
	source, exists := manifest.Sources[command.Source]
	if !exists {
		return invalidMutation(projectdomain.MutationNotFound, "grpc_client.source", command.Source)
	}
	if source.Type == projectdomain.SourceDevctl && (command.Export == "" || command.Path != "") {
		return invalidMutation(projectdomain.MutationInvalidOptions, "grpc_client", command.Name)
	}
	if source.Type != projectdomain.SourceDevctl && (command.Path == "" || command.Export != "") {
		return invalidMutation(projectdomain.MutationInvalidOptions, "grpc_client", command.Name)
	}
	desired := projectdomain.GRPCClient{
		Name:         command.Name,
		Source:       command.Source,
		Export:       command.Export,
		Path:         command.Path,
		ProtoRoot:    command.ProtoRoot,
		BufGenConfig: command.BufGenConfig,
		AddrEnv:      command.AddrEnv,
	}
	if manifest.Components.GRPC == nil {
		manifest.Components.GRPC = &projectdomain.GRPC{}
	}
	for index, current := range manifest.Components.GRPC.Clients {
		if current.Name != desired.Name {
			continue
		}
		if !reflect.DeepEqual(current, desired) && !command.Force {
			return mutationConflict("grpc_client", desired.Name)
		}
		manifest.Components.GRPC.Clients[index] = desired
		return nil
	}
	manifest.Components.GRPC.Clients = append(manifest.Components.GRPC.Clients, desired)
	if manifest.Languages.Go.Generators.GRPC == nil {
		manifest.Languages.Go.Generators.GRPC = &projectdomain.GRPCGenerator{Out: "gen/grpc", BufGenConfig: "tools/buf/grpc.gen.yaml"}
	}
	return nil
}

func kafkaContract(contract projectdomain.KafkaContract) projectdomain.KafkaContract {
	if contract.Format == "" {
		contract.Format = "raw"
	}
	if contract.Format == "proto" {
		if contract.ProtoRoot == "" {
			contract.ProtoRoot = path.Dir(contract.Path)
		}
		if contract.Encoding == "" {
			contract.Encoding = "binary"
		}
	}
	return contract
}

func validateKafkaContract(manifest *projectdomain.Manifest, contract projectdomain.KafkaContract) error {
	if contract.Format != "raw" && contract.Format != "json" && contract.Format != "proto" {
		return invalidMutation(projectdomain.MutationUnsupportedValue, "kafka.format", contract.Format)
	}
	if contract.Format == "raw" && (contract.Source != "" || contract.Export != "" || contract.Path != "" || contract.ProtoRoot != "" || contract.Message != "" || contract.Encoding != "") {
		return invalidMutation(projectdomain.MutationInvalidOptions, "kafka.contract", contract.Format)
	}
	if contract.Format == "raw" {
		return nil
	}
	source, exists := manifest.Sources[contract.Source]
	if !exists {
		return invalidMutation(projectdomain.MutationNotFound, "kafka.source", contract.Source)
	}
	if source.Type == projectdomain.SourceDevctl && (contract.Export == "" || contract.Path != "") {
		return invalidMutation(projectdomain.MutationInvalidOptions, "kafka.contract", contract.Format)
	}
	if source.Type != projectdomain.SourceDevctl && (contract.Path == "" || contract.Export != "") {
		return invalidMutation(projectdomain.MutationInvalidOptions, "kafka.contract", contract.Format)
	}
	if contract.Format == "json" && (contract.ProtoRoot != "" || contract.Message != "" || contract.Encoding != "") {
		return invalidMutation(projectdomain.MutationInvalidOptions, "kafka.contract", contract.Format)
	}
	if contract.Format == "proto" && (contract.Encoding != "binary" && contract.Encoding != "json" || !pathWithin(contract.ProtoRoot, contract.Path)) {
		return invalidMutation(projectdomain.MutationInvalidOptions, "kafka.contract", contract.Format)
	}
	return nil
}

func addKafkaConsumer(manifest *projectdomain.Manifest, command projectdomain.AddKafkaConsumerCommand) error {
	if !kebabCase.MatchString(command.Name) || command.Topic == "" {
		return invalidMutation(projectdomain.MutationInvalidOptions, "kafka_consumer", command.Name)
	}
	contract := kafkaContract(projectdomain.KafkaContract{
		Source: command.Source, Export: command.Export, Path: command.Path, Format: command.Format,
		ProtoRoot: command.ProtoRoot, Message: command.Message, Encoding: command.Encoding,
	})
	if err := validateKafkaContract(manifest, contract); err != nil {
		return err
	}
	desired := projectdomain.KafkaConsumer{
		Name: command.Name, Topic: command.Topic, GroupEnv: command.GroupEnv,
		Start:    desiredStart(command.Always, "KAFKA_"+strings.ToUpper(strings.ReplaceAll(command.Name, "-", "_"))+"_CONSUMER_ENABLED", false),
		Contract: contract,
	}
	if manifest.Components.Kafka == nil {
		manifest.Components.Kafka = &projectdomain.Kafka{}
	}
	for index, current := range manifest.Components.Kafka.Consumers {
		if current.Name == desired.Name {
			if !reflect.DeepEqual(current, desired) && !command.Force {
				return mutationConflict("kafka_consumer", desired.Name)
			}
			manifest.Components.Kafka.Consumers[index] = desired
			return nil
		}
	}
	manifest.Components.Kafka.Consumers = append(manifest.Components.Kafka.Consumers, desired)
	if contract.Format == "proto" && manifest.Languages.Go.Generators.Kafka == nil {
		manifest.Languages.Go.Generators.Kafka = &projectdomain.KafkaGenerator{Out: "gen/kafka", BufGenConfig: "tools/buf/kafka.gen.yaml"}
	}
	return nil
}

func addKafkaProducer(manifest *projectdomain.Manifest, command projectdomain.AddKafkaProducerCommand) error {
	if !kebabCase.MatchString(command.Name) || command.Topic == "" {
		return invalidMutation(projectdomain.MutationInvalidOptions, "kafka_producer", command.Name)
	}
	contract := kafkaContract(projectdomain.KafkaContract{
		Source: command.Source, Export: command.Export, Path: command.Path, Format: command.Format,
		ProtoRoot: command.ProtoRoot, Message: command.Message, Encoding: command.Encoding,
	})
	if err := validateKafkaContract(manifest, contract); err != nil {
		return err
	}
	desired := projectdomain.KafkaProducer{
		Name: command.Name, Topic: command.Topic, TopicEnv: command.TopicEnv, Contract: contract,
	}
	if manifest.Components.Kafka == nil {
		manifest.Components.Kafka = &projectdomain.Kafka{}
	}
	for index, current := range manifest.Components.Kafka.Producers {
		if current.Name == desired.Name {
			if !reflect.DeepEqual(current, desired) && !command.Force {
				return mutationConflict("kafka_producer", desired.Name)
			}
			manifest.Components.Kafka.Producers[index] = desired
			return nil
		}
	}
	manifest.Components.Kafka.Producers = append(manifest.Components.Kafka.Producers, desired)
	if contract.Format == "proto" && manifest.Languages.Go.Generators.Kafka == nil {
		manifest.Languages.Go.Generators.Kafka = &projectdomain.KafkaGenerator{Out: "gen/kafka", BufGenConfig: "tools/buf/kafka.gen.yaml"}
	}
	return nil
}

func addRedis(manifest *projectdomain.Manifest, command projectdomain.AddRedisCommand) error {
	if !kebabCase.MatchString(command.Name) {
		return invalidMutation(projectdomain.MutationInvalidName, "redis.name", command.Name)
	}
	addrEnv := command.AddrEnv
	if addrEnv == "" {
		addrEnv = "REDIS_" + strings.ToUpper(strings.ReplaceAll(command.Name, "-", "_")) + "_ADDR"
	}
	addrDefault := command.AddrDefault
	if addrDefault == "" {
		addrDefault = "localhost:6379"
	}
	if !environmentKey.MatchString(addrEnv) {
		return invalidMutation(projectdomain.MutationInvalidOptions, "redis.addr_env", addrEnv)
	}
	if !validRedisAddress(addrDefault) {
		return invalidMutation(projectdomain.MutationInvalidURL, "redis.addr_default", addrDefault)
	}
	desired := projectdomain.RedisConnection{Name: command.Name, AddrEnv: addrEnv, AddrDefault: addrDefault}
	if manifest.Components.Redis == nil {
		manifest.Components.Redis = &projectdomain.Redis{}
	}
	for index, current := range manifest.Components.Redis.Connections {
		if current.Name != desired.Name {
			continue
		}
		if !reflect.DeepEqual(current, desired) && !command.Force {
			return mutationConflict("redis", desired.Name)
		}
		manifest.Components.Redis.Connections[index] = desired
		return nil
	}
	manifest.Components.Redis.Connections = append(manifest.Components.Redis.Connections, desired)
	return nil
}

func addS3Connection(manifest *projectdomain.Manifest, command projectdomain.AddS3ConnectionCommand) error {
	if !kebabCase.MatchString(command.Name) {
		return invalidMutation(projectdomain.MutationInvalidName, "s3_connection.name", command.Name)
	}
	credentials := command.Credentials
	if credentials == "" {
		credentials = "ambient"
	}
	if credentials != "ambient" && credentials != "static" {
		return invalidMutation(projectdomain.MutationUnsupportedValue, "s3_connection.credentials", credentials)
	}
	desired := projectdomain.S3Connection{Name: command.Name, Credentials: credentials}
	if manifest.Components.S3 == nil {
		manifest.Components.S3 = &projectdomain.S3{}
	}
	for index, current := range manifest.Components.S3.Connections {
		if current.Name != desired.Name {
			continue
		}
		if !reflect.DeepEqual(current, desired) && !command.Force {
			return mutationConflict("s3_connection", desired.Name)
		}
		manifest.Components.S3.Connections[index] = desired
		return nil
	}
	manifest.Components.S3.Connections = append(manifest.Components.S3.Connections, desired)
	return nil
}

func addS3(manifest *projectdomain.Manifest, command projectdomain.AddS3Command) error {
	if !kebabCase.MatchString(command.Name) {
		return invalidMutation(projectdomain.MutationInvalidName, "s3.name", command.Name)
	}
	if manifest.Components.S3 == nil {
		manifest.Components.S3 = &projectdomain.S3{}
	}
	connection := command.Connection
	if connection == "" {
		connection = "default"
		if !hasS3Connection(manifest.Components.S3, connection) {
			manifest.Components.S3.Connections = append(manifest.Components.S3.Connections, projectdomain.S3Connection{
				Name: connection, Credentials: "static", Endpoint: "http://localhost:9000",
				Region: "us-east-1", PathStyle: true,
			})
		}
	}
	if !hasS3Connection(manifest.Components.S3, connection) {
		return invalidMutation(projectdomain.MutationNotFound, "s3.connection", connection)
	}
	desired := projectdomain.S3Bucket{Name: command.Name, Connection: connection, Bucket: command.Name + "-local"}
	for index, current := range manifest.Components.S3.Buckets {
		if current.Name != desired.Name {
			continue
		}
		if !reflect.DeepEqual(current, desired) && !command.Force {
			return mutationConflict("s3", desired.Name)
		}
		manifest.Components.S3.Buckets[index] = desired
		return nil
	}
	manifest.Components.S3.Buckets = append(manifest.Components.S3.Buckets, desired)
	return nil
}

func hasS3Connection(storage *projectdomain.S3, name string) bool {
	for _, connection := range storage.Connections {
		if connection.Name == name {
			return true
		}
	}
	return false
}

func safeRelative(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") {
		return false
	}
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func pathWithin(root, selected string) bool {
	root = path.Clean(strings.ReplaceAll(root, "\\", "/"))
	selected = path.Clean(strings.ReplaceAll(selected, "\\", "/"))
	if root == "." {
		return safeRelative(selected)
	}
	return selected == root || strings.HasPrefix(selected, root+"/")
}

func validRedisAddress(value string) bool {
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		return err == nil && (parsed.Scheme == "redis" || parsed.Scheme == "rediss") &&
			parsed.Hostname() != "" && parsed.User == nil
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || host == "" {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number > 0 && number <= 65535
}

func invalidMutation(reason projectdomain.MutationReason, field, value string) error {
	return &projectdomain.MutationError{Reason: reason, Field: field, Value: value}
}

func mutationConflict(field, value string) error {
	return &projectdomain.MutationError{Reason: projectdomain.MutationExistingConflict, Field: field, Value: value, Conflict: true}
}
