package manifest

import projectdomain "github.com/devctllabs/devctl/internal/domain/project"

func toProjectSpec(document document) projectdomain.Manifest {
	sources := make(map[string]projectdomain.Source, len(document.Sources))
	for name, source := range document.Sources {
		sources[name] = projectdomain.Source{Type: projectdomain.SourceType(source.Type), Path: source.Path, URL: source.URL, Filename: source.Filename, AllowInsecureHTTP: source.AllowInsecureHTTP, Repo: source.Repo, Ref: source.Ref, Proto: projectdomain.SourceProto{BufConfig: source.Proto.BufConfig}}
	}
	exports := make(map[string]projectdomain.Export, len(document.Exports))
	for name, export := range document.Exports {
		exports[name] = projectdomain.Export(export)
	}

	return projectdomain.Manifest{
		Version:    document.Version,
		Project:    projectdomain.Identity(document.Project),
		Env:        mapEnv(document.Env),
		Paths:      projectdomain.ManifestPaths(document.Paths),
		Sources:    sources,
		Exports:    exports,
		Components: mapComponents(document.Components),
		Languages:  mapLanguages(document.Languages),
	}
}

func mapEnv(value envDocument) projectdomain.Env {
	groups := make([]projectdomain.EnvGroup, len(value.Custom))
	for index, group := range value.Custom {
		groups[index] = projectdomain.EnvGroup{Group: group.Group, Vars: mapEnvVars(group.Vars)}
	}
	return projectdomain.Env{Prefix: value.Prefix, Custom: groups}
}

func mapEnvVars(values []envVarDocument) []projectdomain.EnvVar {
	result := make([]projectdomain.EnvVar, len(values))
	for index, value := range values {
		result[index] = projectdomain.EnvVar(value)
	}
	return result
}

func mapComponentEnv(value componentEnvDocument) projectdomain.ComponentEnv {
	return projectdomain.ComponentEnv{System: mapEnvVars(value.System), Custom: mapEnvVars(value.Custom)}
}

func mapStart(value *startDocument) *projectdomain.Start {
	if value == nil {
		return nil
	}
	return &projectdomain.Start{Env: value.Env, Default: value.Default}
}

func mapComponents(value componentsDocument) projectdomain.Components {
	return projectdomain.Components{
		HTTP: mapHTTP(value.HTTP), GRPC: mapGRPC(value.GRPC), Kafka: mapKafka(value.Kafka),
		Logging: mapLogging(value.Logging), Health: mapHealth(value.Health), Telemetry: mapTelemetry(value.Telemetry),
		DB: mapDB(value.DB), Redis: mapRedis(value.Redis), S3: mapS3(value.S3),
	}
}

func mapHTTP(value *httpDocument) *projectdomain.HTTP {
	if value == nil {
		return nil
	}
	clients := make([]projectdomain.HTTPClient, len(value.Clients))
	for index, client := range value.Clients {
		clients[index] = projectdomain.HTTPClient(client)
	}
	var server *projectdomain.HTTPServer
	if value.Server != nil {
		server = &projectdomain.HTTPServer{OpenAPI: value.Server.OpenAPI, Start: mapStart(value.Server.Start)}
	}
	return &projectdomain.HTTP{Server: server, Clients: clients, Env: mapComponentEnv(value.Env)}
}

func mapGRPC(value *grpcDocument) *projectdomain.GRPC {
	if value == nil {
		return nil
	}
	clients := make([]projectdomain.GRPCClient, len(value.Clients))
	for index, client := range value.Clients {
		clients[index] = projectdomain.GRPCClient(client)
	}
	var server *projectdomain.GRPCServer
	if value.Server != nil {
		server = &projectdomain.GRPCServer{ProtoRoot: value.Server.ProtoRoot, BufConfig: value.Server.BufConfig, Start: mapStart(value.Server.Start)}
	}
	return &projectdomain.GRPC{Server: server, Clients: clients, Env: mapComponentEnv(value.Env)}
}

func mapKafka(value *kafkaDocument) *projectdomain.Kafka {
	if value == nil {
		return nil
	}
	consumers := make([]projectdomain.KafkaConsumer, len(value.Consumers))
	for index, consumer := range value.Consumers {
		consumers[index] = projectdomain.KafkaConsumer{Name: consumer.Name, Topic: consumer.Topic, GroupEnv: consumer.GroupEnv, Start: mapStart(consumer.Start), Contract: projectdomain.KafkaContract(consumer.Contract)}
	}
	producers := make([]projectdomain.KafkaProducer, len(value.Producers))
	for index, producer := range value.Producers {
		producers[index] = projectdomain.KafkaProducer{Name: producer.Name, Topic: producer.Topic, TopicEnv: producer.TopicEnv, Contract: projectdomain.KafkaContract(producer.Contract)}
	}
	return &projectdomain.Kafka{Consumers: consumers, Producers: producers, Env: mapComponentEnv(value.Env)}
}

func mapLogging(value *loggingDocument) *projectdomain.Logging {
	if value == nil {
		return nil
	}
	return &projectdomain.Logging{Env: mapComponentEnv(value.Env)}
}

func mapHealth(value *healthDocument) *projectdomain.Health {
	if value == nil {
		return nil
	}
	var server *projectdomain.HealthServer
	if value.Server != nil {
		server = &projectdomain.HealthServer{Start: mapStart(value.Server.Start)}
	}
	return &projectdomain.Health{Server: server, Env: mapComponentEnv(value.Env)}
}

func mapTelemetry(value *telemetryDocument) *projectdomain.Telemetry {
	if value == nil {
		return nil
	}
	return &projectdomain.Telemetry{Start: mapStart(value.Start), Env: mapComponentEnv(value.Env)}
}

func mapDB(value *dbDocument) *projectdomain.DB {
	if value == nil {
		return nil
	}
	connections := make([]projectdomain.DBConnection, len(value.Connections))
	for index, connection := range value.Connections {
		variants := make([]projectdomain.DBVariant, len(connection.Variants))
		for variantIndex, variant := range connection.Variants {
			variants[variantIndex] = mapDBVariant(variant)
		}
		connections[index] = projectdomain.DBConnection{Name: connection.Name, Default: connection.Default, KindEnv: connection.KindEnv, Variants: variants}
	}
	return &projectdomain.DB{Connections: connections, Env: mapComponentEnv(value.Env)}
}

func mapDBVariant(value dbVariantDocument) projectdomain.DBVariant {
	variant := projectdomain.DBVariant{Name: value.Name, Kind: value.Kind, DSNEnv: value.DSNEnv, DSNDefault: value.DSNDefault, Secret: value.Secret}
	if value.Migrations != nil {
		variant.Migrations = &projectdomain.DBMigrations{Path: value.Migrations.Path, DatabaseEnv: value.Migrations.DatabaseEnv, DatabaseDefault: value.Migrations.DatabaseDefault}
	}
	return variant
}

func mapRedis(value *redisDocument) *projectdomain.Redis {
	if value == nil {
		return nil
	}
	connections := make([]projectdomain.RedisConnection, len(value.Connections))
	for index, connection := range value.Connections {
		connections[index] = projectdomain.RedisConnection(connection)
	}
	return &projectdomain.Redis{Connections: connections, Env: mapComponentEnv(value.Env)}
}

func mapS3(value *s3Document) *projectdomain.S3 {
	if value == nil {
		return nil
	}
	connections := make([]projectdomain.S3Connection, len(value.Connections))
	for index, connection := range value.Connections {
		connections[index] = projectdomain.S3Connection(connection)
	}
	buckets := make([]projectdomain.S3Bucket, len(value.Buckets))
	for index, bucket := range value.Buckets {
		buckets[index] = projectdomain.S3Bucket(bucket)
	}
	return &projectdomain.S3{Connections: connections, Buckets: buckets, Env: mapComponentEnv(value.Env)}
}

func mapLanguages(value languagesDocument) projectdomain.Languages {
	generators := projectdomain.GoGenerators{}
	if value.Go.Generators.Config != nil {
		config := projectdomain.ConfigGenerator(*value.Go.Generators.Config)
		generators.Config = &config
	}
	if value.Go.Generators.HTTP != nil {
		http := projectdomain.HTTPGenerator(*value.Go.Generators.HTTP)
		generators.HTTP = &http
	}
	if value.Go.Generators.GRPC != nil {
		grpc := projectdomain.GRPCGenerator(*value.Go.Generators.GRPC)
		generators.GRPC = &grpc
	}
	if value.Go.Generators.Kafka != nil {
		kafka := projectdomain.KafkaGenerator(*value.Go.Generators.Kafka)
		generators.Kafka = &kafka
	}
	components := projectdomain.GoComponents{}
	if value.Go.Components.Pprof != nil {
		var server *projectdomain.PprofServer
		if value.Go.Components.Pprof.Server != nil {
			server = &projectdomain.PprofServer{Start: mapStart(value.Go.Components.Pprof.Server.Start)}
		}
		components.Pprof = &projectdomain.Pprof{Server: server, Env: mapComponentEnv(value.Go.Components.Pprof.Env)}
	}
	return projectdomain.Languages{Go: projectdomain.GoLanguage{Module: value.Go.Module, Generators: generators, Components: components}}
}

func fromProjectManifest(value projectdomain.Manifest) document {
	sources := make(map[string]sourceDocument, len(value.Sources))
	for name, source := range value.Sources {
		sources[name] = sourceDocument{Type: string(source.Type), Path: source.Path, URL: source.URL, Filename: source.Filename, AllowInsecureHTTP: source.AllowInsecureHTTP, Repo: source.Repo, Ref: source.Ref, Proto: sourceProtoDocument{BufConfig: source.Proto.BufConfig}}
	}
	exports := make(map[string]exportDocument, len(value.Exports))
	for name, exported := range value.Exports {
		exports[name] = exportDocument(exported)
	}
	return document{
		Version: value.Version, Project: projectDocument(value.Project), Env: fromProjectEnv(value.Env),
		Paths: pathsDocument(value.Paths), Sources: sources, Exports: exports,
		Components: fromProjectComponents(value.Components), Languages: fromProjectLanguages(value.Languages),
	}
}

func fromProjectEnv(value projectdomain.Env) envDocument {
	groups := make([]envGroupDocument, len(value.Custom))
	for index, group := range value.Custom {
		groups[index] = envGroupDocument{Group: group.Group, Vars: fromProjectEnvVars(group.Vars)}
	}
	return envDocument{Prefix: value.Prefix, Custom: groups}
}

func fromProjectEnvVars(values []projectdomain.EnvVar) []envVarDocument {
	result := make([]envVarDocument, len(values))
	for index, value := range values {
		result[index] = envVarDocument(value)
	}
	return result
}

func fromProjectComponentEnv(value projectdomain.ComponentEnv) componentEnvDocument {
	return componentEnvDocument{System: fromProjectEnvVars(value.System), Custom: fromProjectEnvVars(value.Custom)}
}

func fromProjectStart(value *projectdomain.Start) *startDocument {
	if value == nil {
		return nil
	}
	return &startDocument{Env: value.Env, Default: value.Default}
}

func fromProjectComponents(value projectdomain.Components) componentsDocument {
	return componentsDocument{
		HTTP: fromProjectHTTP(value.HTTP), GRPC: fromProjectGRPC(value.GRPC), Kafka: fromProjectKafka(value.Kafka),
		Logging: fromProjectLogging(value.Logging), Health: fromProjectHealth(value.Health), Telemetry: fromProjectTelemetry(value.Telemetry),
		DB: fromProjectDB(value.DB), Redis: fromProjectRedis(value.Redis), S3: fromProjectS3(value.S3),
	}
}

func fromProjectHTTP(value *projectdomain.HTTP) *httpDocument {
	if value == nil {
		return nil
	}
	clients := make([]httpClientDocument, len(value.Clients))
	for index, client := range value.Clients {
		clients[index] = httpClientDocument(client)
	}
	var server *httpServerDocument
	if value.Server != nil {
		server = &httpServerDocument{OpenAPI: value.Server.OpenAPI, Start: fromProjectStart(value.Server.Start)}
	}
	return &httpDocument{Server: server, Clients: clients, Env: fromProjectComponentEnv(value.Env)}
}

func fromProjectGRPC(value *projectdomain.GRPC) *grpcDocument {
	if value == nil {
		return nil
	}
	clients := make([]grpcClientDocument, len(value.Clients))
	for index, client := range value.Clients {
		clients[index] = grpcClientDocument(client)
	}
	var server *grpcServerDocument
	if value.Server != nil {
		server = &grpcServerDocument{ProtoRoot: value.Server.ProtoRoot, BufConfig: value.Server.BufConfig, Start: fromProjectStart(value.Server.Start)}
	}
	return &grpcDocument{Server: server, Clients: clients, Env: fromProjectComponentEnv(value.Env)}
}

func fromProjectKafka(value *projectdomain.Kafka) *kafkaDocument {
	if value == nil {
		return nil
	}
	consumers := make([]kafkaConsumerDocument, len(value.Consumers))
	for index, consumer := range value.Consumers {
		consumers[index] = kafkaConsumerDocument{Name: consumer.Name, Topic: consumer.Topic, GroupEnv: consumer.GroupEnv, Start: fromProjectStart(consumer.Start), Contract: kafkaContractDocument(consumer.Contract)}
	}
	producers := make([]kafkaProducerDocument, len(value.Producers))
	for index, producer := range value.Producers {
		producers[index] = kafkaProducerDocument{Name: producer.Name, Topic: producer.Topic, TopicEnv: producer.TopicEnv, Contract: kafkaContractDocument(producer.Contract)}
	}
	return &kafkaDocument{Consumers: consumers, Producers: producers, Env: fromProjectComponentEnv(value.Env)}
}

func fromProjectLogging(value *projectdomain.Logging) *loggingDocument {
	if value == nil {
		return nil
	}
	return &loggingDocument{Env: fromProjectComponentEnv(value.Env)}
}

func fromProjectHealth(value *projectdomain.Health) *healthDocument {
	if value == nil {
		return nil
	}
	var server *healthServerDocument
	if value.Server != nil {
		server = &healthServerDocument{Start: fromProjectStart(value.Server.Start)}
	}
	return &healthDocument{Server: server, Env: fromProjectComponentEnv(value.Env)}
}

func fromProjectTelemetry(value *projectdomain.Telemetry) *telemetryDocument {
	if value == nil {
		return nil
	}
	return &telemetryDocument{Start: fromProjectStart(value.Start), Env: fromProjectComponentEnv(value.Env)}
}

func fromProjectDB(value *projectdomain.DB) *dbDocument {
	if value == nil {
		return nil
	}
	connections := make([]dbConnectionDocument, len(value.Connections))
	for index, connection := range value.Connections {
		variants := make([]dbVariantDocument, len(connection.Variants))
		for variantIndex, variant := range connection.Variants {
			variants[variantIndex] = fromProjectDBVariant(variant)
		}
		connections[index] = dbConnectionDocument{Name: connection.Name, Default: connection.Default, KindEnv: connection.KindEnv, Variants: variants}
	}
	return &dbDocument{Connections: connections, Env: fromProjectComponentEnv(value.Env)}
}

func fromProjectDBVariant(value projectdomain.DBVariant) dbVariantDocument {
	variant := dbVariantDocument{Name: value.Name, Kind: value.Kind, DSNEnv: value.DSNEnv, DSNDefault: value.DSNDefault, Secret: value.Secret}
	if value.Migrations != nil {
		variant.Migrations = &dbMigrationsDocument{Path: value.Migrations.Path, DatabaseEnv: value.Migrations.DatabaseEnv, DatabaseDefault: value.Migrations.DatabaseDefault}
	}
	return variant
}

func fromProjectRedis(value *projectdomain.Redis) *redisDocument {
	if value == nil {
		return nil
	}
	connections := make([]redisConnectionDocument, len(value.Connections))
	for index, connection := range value.Connections {
		connections[index] = redisConnectionDocument(connection)
	}
	return &redisDocument{Connections: connections, Env: fromProjectComponentEnv(value.Env)}
}

func fromProjectS3(value *projectdomain.S3) *s3Document {
	if value == nil {
		return nil
	}
	connections := make([]s3ConnectionDocument, len(value.Connections))
	for index, connection := range value.Connections {
		connections[index] = s3ConnectionDocument(connection)
	}
	buckets := make([]s3BucketDocument, len(value.Buckets))
	for index, bucket := range value.Buckets {
		buckets[index] = s3BucketDocument(bucket)
	}
	return &s3Document{Connections: connections, Buckets: buckets, Env: fromProjectComponentEnv(value.Env)}
}

func fromProjectLanguages(value projectdomain.Languages) languagesDocument {
	generators := goGeneratorsDocument{}
	if value.Go.Generators.Config != nil {
		config := configGeneratorDocument(*value.Go.Generators.Config)
		generators.Config = &config
	}
	if value.Go.Generators.HTTP != nil {
		http := httpGeneratorDocument(*value.Go.Generators.HTTP)
		generators.HTTP = &http
	}
	if value.Go.Generators.GRPC != nil {
		grpc := grpcGeneratorDocument(*value.Go.Generators.GRPC)
		generators.GRPC = &grpc
	}
	if value.Go.Generators.Kafka != nil {
		kafka := kafkaGeneratorDocument(*value.Go.Generators.Kafka)
		generators.Kafka = &kafka
	}
	components := goComponentsDocument{}
	if value.Go.Components.Pprof != nil {
		var server *pprofServerDocument
		if value.Go.Components.Pprof.Server != nil {
			server = &pprofServerDocument{Start: fromProjectStart(value.Go.Components.Pprof.Server.Start)}
		}
		components.Pprof = &pprofDocument{Server: server, Env: fromProjectComponentEnv(value.Go.Components.Pprof.Env)}
	}
	return languagesDocument{Go: goLanguageDocument{Module: value.Go.Module, Generators: generators, Components: components}}
}
