package scaffold

import (
	"path"
	"sort"
	"strings"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

type scaffoldProjection struct {
	projectName string
	module      string
	hasServer   bool
	container   containerTemplateData
	application applicationTemplateData
	runtime     runtimeTemplateData
	config      configProjection
	http        httpProjection
	proto       protoProjection
	components  componentProjection
	storages    []storageArtifactProjection
	hasSQLite   bool
	goMod       goModTemplateData
	mise        miseTemplateData
	main        mainTemplateData
	seedPaths   map[string]struct{}
}

func compileScaffoldProjection(manifest projectdomain.Manifest) scaffoldProjection {
	hasServer := manifestHasServer(manifest)
	kafkaProto := manifestHasKafkaFormat(manifest, "proto")
	kafkaJSON := manifestHasKafkaFormat(manifest, "json")
	targets := projectdomain.NewTargetCatalog(manifest)
	runtimeCatalog, runtimeErr := projectdomain.NewRuntimeConfigCatalog(manifest)
	configTargets := targets.Select(projectdomain.TargetOperationGenerate, "config", "")
	config := configProjection{catalog: runtimeCatalog, catalogErr: runtimeErr}
	if len(configTargets) == 1 {
		config.targetAvailable = true
		config.target = configTargets[0]
		config.importPath = path.Join(manifest.Languages.Go.Module, configTargets[0].OutputDir)
	}
	httpTargets := targets.Select(projectdomain.TargetOperationGenerate, "http", "")
	proto := compileProtoProjection(manifest, targets, kafkaProto)
	components := compileComponentProjection(manifest)
	storages, hasSQLite := compileStorageProjections(manifest)
	projection := scaffoldProjection{
		projectName: manifest.Project.Name,
		module:      manifest.Languages.Go.Module,
		hasServer:   hasServer,
		container:   compileContainerTemplate(manifest, hasServer),
		application: compileApplicationTemplate(manifest, hasServer),
		runtime:     compileRuntimeTemplate(manifest),
		config:      config,
		http:        httpProjection{enabled: manifest.Components.HTTP != nil, targets: httpTargets},
		proto:       proto,
		components:  components,
		storages:    storages,
		hasSQLite:   hasSQLite,
		goMod:       compileGoModTemplate(manifest, hasServer, kafkaProto),
		mise:        compileMiseTemplate(manifest, kafkaJSON),
		main: mainTemplateData{
			Project: manifest.Project.Name, Module: manifest.Languages.Go.Module,
			Server: hasServer, Kafka: manifest.Components.Kafka != nil,
		},
	}
	projection.seedPaths = compileSeedPaths(projection)
	return projection
}

func compileSeedPaths(projection scaffoldProjection) map[string]struct{} {
	paths := map[string]struct{}{
		"README.md":                    {},
		"internal/deps/application.go": {},
		path.Join("cmd", projection.projectName, "main.go"): {},
	}
	if projection.hasServer {
		paths[path.Join("cmd", projection.projectName, "internal", "api.go")] = struct{}{}
	}
	for _, target := range projection.http.targets {
		if target.Role == "server" {
			paths[target.Reference.Entrypoint] = struct{}{}
		}
	}
	if kafka := projection.components.kafka; kafka != nil {
		paths[path.Join("cmd", kafka.projectName, "internal", "consumer.go")] = struct{}{}
		for _, fact := range kafka.consumerSeedFacts {
			paths[path.Join("internal", "deps", "consumer_"+fact.packageName+".go")] = struct{}{}
			paths[path.Join("internal", "transport", "consumerkafka", fact.packageName, "handler.go")] = struct{}{}
		}
	}
	for _, storage := range projection.storages {
		for _, migrationPath := range storage.migrationPaths {
			paths[path.Join(migrationPath, ".gitkeep")] = struct{}{}
		}
	}
	if projection.hasSQLite {
		paths["data/.gitkeep"] = struct{}{}
	}
	return paths
}

func (p scaffoldProjection) scaffoldSeed(artifactPath string) bool {
	_, exists := p.seedPaths[canonicalArtifactPath(artifactPath)]
	return exists
}

type componentProjection struct {
	grpcEnabled      bool
	grpcClients      []projectdomain.GRPCClient
	httpClients      []projectdomain.HTTPClient
	redisEnabled     bool
	redisConnections []projectdomain.RedisConnection
	s3               *projectdomain.S3
	kafka            *kafkaProjection
}

type kafkaProjection struct {
	module            string
	projectName       string
	consumers         []kafkaConsumerTemplateData
	producers         []projectdomain.KafkaProducer
	consumerSeedFacts []kafkaConsumerSeedFact
}

type kafkaConsumerSeedFact struct {
	packageName string
	data        kafkaConsumerTemplateData
}

func compileComponentProjection(manifest projectdomain.Manifest) componentProjection {
	projection := componentProjection{
		grpcEnabled:  manifest.Components.GRPC != nil,
		redisEnabled: manifest.Components.Redis != nil,
	}
	if manifest.Components.GRPC != nil {
		projection.grpcClients = append([]projectdomain.GRPCClient(nil), manifest.Components.GRPC.Clients...)
	}
	if manifest.Components.HTTP != nil {
		projection.httpClients = append([]projectdomain.HTTPClient(nil), manifest.Components.HTTP.Clients...)
	}
	if manifest.Components.Redis != nil {
		projection.redisConnections = append([]projectdomain.RedisConnection(nil), manifest.Components.Redis.Connections...)
	}
	if manifest.Components.S3 != nil {
		storage := *manifest.Components.S3
		storage.Connections = append([]projectdomain.S3Connection(nil), storage.Connections...)
		storage.Buckets = append([]projectdomain.S3Bucket(nil), storage.Buckets...)
		projection.s3 = &storage
	}
	if manifest.Components.Kafka != nil {
		kafka := &kafkaProjection{
			module:      manifest.Languages.Go.Module,
			projectName: manifest.Project.Name,
			producers:   append([]projectdomain.KafkaProducer(nil), manifest.Components.Kafka.Producers...),
		}
		kafka.consumers = make([]kafkaConsumerTemplateData, len(manifest.Components.Kafka.Consumers))
		kafka.consumerSeedFacts = make([]kafkaConsumerSeedFact, len(manifest.Components.Kafka.Consumers))
		for index, consumer := range manifest.Components.Kafka.Consumers {
			data := kafkaConsumerTemplateData{
				Name: consumer.Name, Topic: consumer.Topic, Toggle: consumer.Start != nil,
				Format: consumer.Contract.Format, Module: manifest.Languages.Go.Module,
			}
			kafka.consumers[index] = data
			kafka.consumerSeedFacts[index] = kafkaConsumerSeedFact{
				packageName: strings.ReplaceAll(consumer.Name, "-", "_"), data: data,
			}
		}
		projection.kafka = kafka
	}
	return projection
}

type storageArtifactProjection struct {
	path           string
	template       storageTemplateData
	migrationPaths []string
}

func compileStorageProjections(manifest projectdomain.Manifest) ([]storageArtifactProjection, bool) {
	if manifest.Components.DB == nil {
		return nil, false
	}
	connections := append([]projectdomain.DBConnection(nil), manifest.Components.DB.Connections...)
	sort.Slice(connections, func(i, j int) bool { return connections[i].Name < connections[j].Name })
	projections := make([]storageArtifactProjection, 0, len(connections))
	hasSQLite := false
	for _, connection := range connections {
		projection := storageArtifactProjection{
			path:     "internal/deps/storage_" + strings.ReplaceAll(connection.Name, "-", "_") + ".gen.go",
			template: compileStorageTemplate(connection, manifest.Components.Telemetry != nil),
		}
		for _, variant := range connection.Variants {
			hasSQLite = hasSQLite || variant.Kind == "sqlite"
			if variant.Migrations != nil {
				projection.migrationPaths = append(projection.migrationPaths, variant.Migrations.Path)
			}
		}
		projections = append(projections, projection)
	}
	return projections, hasSQLite
}

func compileStorageTemplate(connection projectdomain.DBConnection, telemetry bool) storageTemplateData {
	data := storageTemplateData{Name: goName(connection.Name), Connection: connection.Name, Telemetry: telemetry}
	seenKinds := make(map[string]bool, len(connection.Variants))
	data.Variants = make([]storageVariant, 0, len(connection.Variants))
	for _, variant := range connection.Variants {
		if variant.Kind == "clickhouse" {
			data.ClickHouse = true
			data.ClickHouseConfig = goName(variant.Name) + "DSN"
			data.Variants = append(data.Variants, storageVariant{
				Name: variant.Name, Kind: variant.Kind,
				ConfigField: goName(variant.Name) + "DSN", ClickHouse: true,
			})
			continue
		}
		field := strings.ToLower(variant.Kind[:1]) + variant.Kind[1:]
		if !seenKinds[variant.Kind] {
			data.Kinds = append(data.Kinds, storageKind{Name: variant.Kind, Field: field})
			seenKinds[variant.Kind] = true
		}
		data.Variants = append(data.Variants, storageVariant{
			Name: variant.Name, Kind: variant.Kind, Field: field,
			ConfigField: goName(variant.Name) + "DSN",
			SQLite:      variant.Kind == "sqlite",
		})
	}
	return data
}

type httpProjection struct {
	enabled bool
	targets []projectdomain.Target
}

type protoProjection struct {
	enabled     bool
	configPaths []string
	grpcModule  *grpcModuleProjection
}

type grpcModuleProjection struct {
	path      string
	protoRoot string
}

func compileProtoProjection(
	manifest projectdomain.Manifest,
	targets projectdomain.TargetCatalog,
	kafkaProto bool,
) protoProjection {
	projection := protoProjection{enabled: manifest.Components.GRPC != nil || kafkaProto}
	seen := make(map[string]struct{}, 2)
	for _, target := range targets.Select(projectdomain.TargetOperationGenerate, "grpc", "") {
		if target.Config == "tools/buf/grpc.gen.yaml" {
			seen[target.Config] = struct{}{}
		}
	}
	for _, target := range targets.Select(projectdomain.TargetOperationGenerate, "kafka", "") {
		if target.Format == "proto" && target.Config == "tools/buf/kafka.gen.yaml" {
			seen[target.Config] = struct{}{}
		}
	}
	projection.configPaths = make([]string, 0, len(seen))
	for configPath := range seen {
		projection.configPaths = append(projection.configPaths, configPath)
	}
	sort.Strings(projection.configPaths)
	if manifest.Components.GRPC != nil && manifest.Components.GRPC.Server != nil {
		server := manifest.Components.GRPC.Server
		projection.grpcModule = &grpcModuleProjection{
			path:      valueOrDefault(server.BufConfig, "buf.yaml"),
			protoRoot: valueOrDefault(server.ProtoRoot, "api/proto/grpc"),
		}
	}
	return projection
}

func compileGoModTemplate(
	manifest projectdomain.Manifest,
	hasServer bool,
	kafkaProto bool,
) goModTemplateData {
	requires := []string{
		"github.com/devctllabs/go-libs/config v0.1.0",
		"github.com/devctllabs/go-libs/di v0.1.0",
		"github.com/urfave/cli/v3 v3.10.1",
	}
	if manifest.Components.Logging != nil {
		requires = append(requires, "github.com/devctllabs/go-libs/log v0.2.0", "go.uber.org/zap v1.28.0")
	}
	if hasServer {
		requires = append(requires, "github.com/devctllabs/go-libs/lifecycle v0.2.0")
	}
	if manifest.Components.HTTP != nil && manifest.Components.HTTP.Server != nil {
		requires = append(requires, "github.com/devctllabs/go-libs/oapivalidator v0.2.0", "github.com/labstack/echo/v5 v5.3.1")
	}
	if manifest.Components.Health != nil {
		requires = append(requires, "github.com/devctllabs/go-libs/health v0.1.0", "github.com/devctllabs/go-libs/healthserver v0.1.0")
	}
	if manifest.Components.Telemetry != nil {
		requires = append(requires, "github.com/devctllabs/go-libs/telemetry v0.1.0")
	}
	if manifest.Languages.Go.Components.Pprof != nil {
		requires = append(requires, "github.com/devctllabs/go-libs/debugserver v0.1.0")
	}
	if manifest.Components.GRPC != nil {
		requires = append(requires,
			"github.com/bufbuild/buf v1.72.0",
			"google.golang.org/grpc v1.83.2",
			"google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.2",
			"google.golang.org/protobuf v1.36.12",
		)
	}
	if manifest.Components.Kafka != nil {
		requires = append(requires,
			"github.com/devctllabs/go-libs/kafka v0.1.0",
			"github.com/devctllabs/go-libs/retry v0.1.0",
			"github.com/twmb/franz-go v1.21.6",
		)
		if manifestHasKafkaConsumerFormat(manifest, "proto") {
			requires = append(requires, "github.com/devctllabs/go-libs/kafkaproto v0.1.0")
		}
	}
	if manifest.Components.Redis != nil {
		requires = append(requires, "github.com/redis/go-redis/v9 v9.22.0")
	}
	if manifest.Components.S3 != nil {
		requires = append(requires,
			"github.com/aws/aws-sdk-go-v2 v1.45.1",
			"github.com/aws/aws-sdk-go-v2/config v1.33.1",
			"github.com/aws/aws-sdk-go-v2/credentials v1.20.1",
			"github.com/aws/aws-sdk-go-v2/service/s3 v1.109.1",
		)
	}
	if manifest.Components.DB != nil {
		for _, connection := range manifest.Components.DB.Connections {
			for _, variant := range connection.Variants {
				requires = append(requires, databaseVariantRequirements(variant.Kind)...)
			}
		}
	}
	sort.Strings(requires)
	return goModTemplateData{
		Module: manifest.Languages.Go.Module, Requires: unique(requires),
		HTTP: manifest.Components.HTTP != nil, Proto: manifest.Components.GRPC != nil || kafkaProto,
	}
}

func databaseVariantRequirements(kind string) []string {
	if kind == "clickhouse" {
		return []string{"github.com/ClickHouse/clickhouse-go/v2 v2.48.0"}
	}
	version := "v0.1.0"
	if kind == "postgres" {
		version = "v0.2.0"
	}
	return []string{
		"github.com/devctllabs/go-libs/txmanager v0.1.0",
		"github.com/devctllabs/go-libs/" + kind + "db " + version,
	}
}

func compileMiseTemplate(manifest projectdomain.Manifest, kafkaJSON bool) miseTemplateData {
	prefix := manifest.Env.Prefix
	if prefix == "" {
		prefix = strings.ToUpper(strings.ReplaceAll(manifest.Project.Name, "-", "_")) + "_"
	}
	migrations, tags := compileMigrationTasks(manifest.Components.DB, prefix)
	return miseTemplateData{JSON: kafkaJSON, Migrations: migrations, MigrationTags: tags}
}

func compileMigrationTasks(database *projectdomain.DB, prefix string) ([]migrationTask, []string) {
	if database == nil {
		return nil, nil
	}
	var migrations []migrationTask
	migrationKinds := make(map[string]struct{})
	for _, connection := range database.Connections {
		connectionTasks := compileConnectionMigrationTasks(connection, prefix)
		migrations = append(migrations, connectionTasks...)
		for _, task := range connectionTasks {
			migrationKinds[task.Kind] = struct{}{}
		}
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].TaskPrefix < migrations[j].TaskPrefix })
	tags := sortedKeys(migrationKinds)
	return migrations, tags
}

func compileConnectionMigrationTasks(connection projectdomain.DBConnection, prefix string) []migrationTask {
	var tasks []migrationTask
	for _, variant := range connection.Variants {
		if task, exists := compileMigrationTask(connection.Name, variant, prefix); exists {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func compileMigrationTask(
	connectionName string,
	variant projectdomain.DBVariant,
	prefix string,
) (migrationTask, bool) {
	if variant.Migrations == nil {
		return migrationTask{}, false
	}
	environment := prefix + variant.Migrations.DatabaseEnv
	expression := "${" + environment + ":?set " + environment + "}"
	if variant.Migrations.DatabaseDefault != "" {
		expression = "${" + environment + ":-" + variant.Migrations.DatabaseDefault + "}"
	}
	return migrationTask{
		TaskPrefix: "migrate:" + connectionName + ":" + variant.Name,
		Path:       variant.Migrations.Path, DatabaseExpression: expression, Kind: variant.Kind,
	}, true
}

func sortedKeys[Value any](values map[string]Value) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func manifestHasKafkaFormat(manifest projectdomain.Manifest, format string) bool {
	if manifest.Components.Kafka == nil {
		return false
	}
	for _, consumer := range manifest.Components.Kafka.Consumers {
		if consumer.Contract.Format == format {
			return true
		}
	}
	for _, producer := range manifest.Components.Kafka.Producers {
		if producer.Contract.Format == format {
			return true
		}
	}
	return false
}

func manifestHasKafkaConsumerFormat(manifest projectdomain.Manifest, format string) bool {
	if manifest.Components.Kafka == nil {
		return false
	}
	for _, consumer := range manifest.Components.Kafka.Consumers {
		if consumer.Contract.Format == format {
			return true
		}
	}
	return false
}

func compileContainerTemplate(manifest projectdomain.Manifest, hasServer bool) containerTemplateData {
	data := containerTemplateData{
		Project:   manifest.Project.Name,
		Logging:   manifest.Components.Logging != nil,
		Telemetry: manifest.Components.Telemetry != nil,
		Database:  manifest.Components.DB != nil,
		Server:    hasServer,
		Kafka:     manifest.Components.Kafka != nil,
	}
	if manifest.Components.Telemetry != nil {
		data.TelemetryToggle = manifest.Components.Telemetry.Start != nil
	}
	if manifest.Components.DB != nil {
		data.RuntimeUsesResolver = manifest.Components.Health != nil && len(manifest.Components.DB.Connections) > 0
		data.Connections = make([]containerConnection, 0, len(manifest.Components.DB.Connections))
		for _, connection := range manifest.Components.DB.Connections {
			data.Connections = append(data.Connections, containerConnection{
				Name: goName(connection.Name), Connection: connection.Name,
			})
		}
	}
	if manifest.Components.Kafka != nil {
		for _, consumer := range manifest.Components.Kafka.Consumers {
			enabled := "true"
			if consumer.Start != nil {
				enabled = "cfg.Kafka." + goName(consumer.Name) + "Enabled"
			}
			data.Consumers = append(data.Consumers, containerConsumer{Name: consumer.Name, Enabled: enabled})
		}
	}
	return data
}

func compileApplicationTemplate(manifest projectdomain.Manifest, hasServer bool) applicationTemplateData {
	return applicationTemplateData{
		HTTP:  manifest.Components.HTTP != nil && manifest.Components.HTTP.Server != nil,
		GRPC:  manifest.Components.GRPC != nil && manifest.Components.GRPC.Server != nil,
		Calls: applicationProviderCalls(manifest, hasServer),
	}
}

func applicationProviderCalls(manifest projectdomain.Manifest, hasServer bool) []string {
	var calls []string
	if manifest.Components.Logging != nil {
		calls = append(calls, "provideLogging")
	}
	if manifest.Components.Telemetry != nil {
		calls = append(calls, "provideTelemetry")
	}
	calls = append(calls, databaseProviderCalls(manifest.Components.DB)...)
	calls = append(calls, kafkaProviderCalls(manifest.Components.Kafka)...)
	calls = append(calls, httpClientProviderCalls(manifest.Components.HTTP)...)
	calls = append(calls, grpcClientProviderCalls(manifest.Components.GRPC)...)
	calls = append(calls, redisProviderCalls(manifest.Components.Redis)...)
	calls = append(calls, s3ProviderCalls(manifest.Components.S3)...)
	if hasServer {
		calls = append(calls, "provideRuntime")
	}
	return calls
}

func databaseProviderCalls(database *projectdomain.DB) []string {
	if database == nil {
		return nil
	}
	calls := make([]string, 0, len(database.Connections))
	for _, connection := range database.Connections {
		calls = append(calls, "provideStorage"+goName(connection.Name))
	}
	return calls
}

func kafkaProviderCalls(broker *projectdomain.Kafka) []string {
	if broker == nil {
		return nil
	}
	calls := make([]string, 0, len(broker.Consumers)+len(broker.Producers))
	for _, consumer := range broker.Consumers {
		calls = append(calls, "provide"+goName(consumer.Name)+"Consumer")
	}
	for _, producer := range broker.Producers {
		calls = append(calls, "provide"+goName(producer.Name)+"KafkaProducer")
	}
	return calls
}

func httpClientProviderCalls(http *projectdomain.HTTP) []string {
	if http == nil {
		return nil
	}
	calls := make([]string, 0, len(http.Clients))
	for _, client := range http.Clients {
		calls = append(calls, "provide"+goName(client.Name)+"HTTPClient")
	}
	return calls
}

func grpcClientProviderCalls(grpc *projectdomain.GRPC) []string {
	if grpc == nil {
		return nil
	}
	calls := make([]string, 0, len(grpc.Clients))
	for _, client := range grpc.Clients {
		calls = append(calls, "provide"+goName(client.Name)+"GRPCClient")
	}
	return calls
}

func redisProviderCalls(redis *projectdomain.Redis) []string {
	if redis == nil {
		return nil
	}
	calls := make([]string, 0, len(redis.Connections))
	for _, connection := range redis.Connections {
		calls = append(calls, "provideRedis"+goName(connection.Name))
	}
	return calls
}

func s3ProviderCalls(storage *projectdomain.S3) []string {
	if storage == nil {
		return nil
	}
	calls := make([]string, 0, len(storage.Connections)+len(storage.Buckets))
	for _, connection := range storage.Connections {
		calls = append(calls, "provideS3"+goName(connection.Name))
	}
	for _, bucket := range storage.Buckets {
		calls = append(calls, "provideS3"+goName(bucket.Name)+"Bucket")
	}
	return calls
}

func compileRuntimeTemplate(manifest projectdomain.Manifest) runtimeTemplateData {
	data := runtimeTemplateData{
		HTTP:   manifest.Components.HTTP != nil && manifest.Components.HTTP.Server != nil,
		GRPC:   manifest.Components.GRPC != nil && manifest.Components.GRPC.Server != nil,
		Health: manifest.Components.Health != nil,
		Pprof:  manifest.Languages.Go.Components.Pprof != nil,
	}
	if data.HTTP {
		data.HTTPToggle = manifest.Components.HTTP.Server.Start != nil
	}
	if data.GRPC {
		data.GRPCToggle = manifest.Components.GRPC.Server.Start != nil
	}
	if data.Health && manifest.Components.Health.Server != nil {
		data.HealthToggle = manifest.Components.Health.Server.Start != nil
	}
	if data.Pprof && manifest.Languages.Go.Components.Pprof.Server != nil {
		data.PprofToggle = manifest.Languages.Go.Components.Pprof.Server.Start != nil
	}
	if data.Health && manifest.Components.DB != nil {
		data.HealthConnections = make([]runtimeHealthConnection, 0, len(manifest.Components.DB.Connections))
		for _, connection := range manifest.Components.DB.Connections {
			data.HealthConnections = append(data.HealthConnections, runtimeHealthConnection{
				Connection: "db-connection:" + connection.Name,
				Probe:      "db." + connection.Name,
			})
		}
	}
	return data
}

func manifestHasServer(manifest projectdomain.Manifest) bool {
	return manifest.Components.HTTP != nil && manifest.Components.HTTP.Server != nil ||
		manifest.Components.GRPC != nil && manifest.Components.GRPC.Server != nil ||
		manifest.Components.Health != nil || manifest.Languages.Go.Components.Pprof != nil
}
