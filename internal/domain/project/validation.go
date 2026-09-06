package project

import (
	"errors"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var validationName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var validationEnvironmentKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

type semanticValidation struct {
	project Project
	issues  []Issue
}

// Validate returns every context-free semantic issue in stable order.
func Validate(project Project) []Issue {
	validation := &semanticValidation{project: project}
	validation.validateIdentityAndPaths()
	validation.validateSources()
	validation.validateExports()
	validation.validateHTTP()
	validation.validateGRPC()
	validation.validateKafka()
	validation.validateComponentPolicies()
	validation.validateRuntimeConfig()
	return validation.issues
}

func (v *semanticValidation) add(code IssueCode, field string) {
	v.issues = append(v.issues, Issue{Code: code, Path: v.project.ManifestPath, Field: field})
}

func (v *semanticValidation) validateIdentityAndPaths() {
	manifest := v.project.Manifest
	if manifest.Version != 1 {
		v.add(IssueVersionUnsupported, "version")
	}
	if !validationName.MatchString(manifest.Project.Name) {
		v.add(IssueNameInvalid, "project.name")
	}
	if manifest.Project.Language != "go" {
		v.add(IssueLanguageUnsupported, "project.language")
	}
	if manifest.Languages.Go.Module == "" {
		v.add(IssueGoModuleRequired, "languages.go.module")
	}
	if manifest.Paths.ExternalContracts != "" && !validationSafeRelative(manifest.Paths.ExternalContracts) {
		v.add(IssuePathInvalid, "paths.external_contracts")
	}
	v.validateOutputPaths()
}

func (v *semanticValidation) validateSources() {
	for _, name := range validationSortedKeys(v.project.Manifest.Sources) {
		v.validateSource(name, v.project.Manifest.Sources[name])
	}
}

func (v *semanticValidation) validateSource(name string, source Source) {
	base := "sources." + name
	if !validationName.MatchString(name) {
		v.add(IssueSourceNameInvalid, base)
	}
	unsafeBufConfig := source.Proto.BufConfig != "" && !validationSafeRelative(source.Proto.BufConfig)
	validatedSource := source
	if unsafeBufConfig {
		validatedSource.Proto.BufConfig = ""
		v.add(IssuePathInvalid, base+".proto.buf_config")
	}
	if !sourceShapeValid(validatedSource) {
		v.add(IssueSourceInvalid, base)
	}
	switch source.Type {
	case SourceLocal:
		if source.Path == "" || !validationSafeRelative(source.Path) {
			v.add(IssueSourceInvalid, base)
		}
	case SourceURL:
		if source.URL == "" {
			v.add(IssueSourceInvalid, base)
		}
		if strings.HasPrefix(strings.ToLower(source.URL), "http://") && !source.AllowInsecureHTTP {
			v.add(IssueSourceInsecure, base)
		}
	case SourceGit, SourceDevctl:
		if source.Repo == "" || source.Ref == "" {
			v.add(IssueSourceInvalid, base)
		}
	default:
		v.add(IssueSourceTypeUnsupported, base)
	}
}

func sourceShapeValid(source Source) bool {
	if source.Proto.BufConfig != "" && !validationSafeRelative(source.Proto.BufConfig) {
		return false
	}
	switch source.Type {
	case SourceLocal:
		return validationSafeRelative(source.Path) && source.URL == "" && source.Repo == "" &&
			source.Ref == "" && source.Filename == "" && !source.AllowInsecureHTTP
	case SourceURL:
		parsed, err := url.Parse(source.URL)
		validURL := err == nil && parsed.Host != "" && parsed.User == nil &&
			(parsed.Scheme == "https" || parsed.Scheme == "http" && source.AllowInsecureHTTP)
		return validURL && source.Path == "" && source.Repo == "" && source.Ref == "" &&
			(source.Filename == "" || !strings.Contains(source.Filename, "/"))
	case SourceGit:
		return source.Repo != "" && source.Ref != "" &&
			(source.Path == "" || validationSafeRelative(source.Path)) && source.URL == "" &&
			source.Filename == "" && !source.AllowInsecureHTTP
	case SourceDevctl:
		return source.Repo != "" && source.Ref != "" && source.Path == "" && source.URL == "" &&
			source.Filename == "" && !source.AllowInsecureHTTP
	default:
		return false
	}
}

func (v *semanticValidation) validateExports() {
	for _, name := range validationSortedKeys(v.project.Manifest.Exports) {
		if !v.project.Manifest.ExportMatchesSurface(v.project.Manifest.Exports[name]) {
			v.add(IssueExportInvalid, "exports."+name)
		}
	}
}

func (v *semanticValidation) validateHTTP() {
	httpComponent := v.project.Manifest.Components.HTTP
	if httpComponent == nil {
		return
	}
	if server := httpComponent.Server; server != nil {
		entrypoint := valueOrDefault(server.OpenAPI, "api/openapi/swagger.yaml")
		if !validationSafeRelative(entrypoint) {
			v.add(IssuePathInvalid, "components.http.server.openapi")
		}
	}
	seen := make(map[string]bool, len(httpComponent.Clients))
	for _, client := range httpComponent.Clients {
		base := "components.http.clients." + client.Name
		if !validationName.MatchString(client.Name) || seen[client.Name] {
			v.add(IssueHTTPClientInvalid, base)
		}
		source, exists := v.project.Manifest.Sources[client.Source]
		if !exists {
			v.add(IssueSourceNotFound, base+".source")
		} else if clientContractSelectionInvalid(source, client.Export, client.Path) {
			v.add(IssueHTTPClientInvalid, base)
		}
		seen[client.Name] = true
	}
}

func (v *semanticValidation) validateGRPC() {
	grpc := v.project.Manifest.Components.GRPC
	if grpc == nil {
		return
	}
	if server := grpc.Server; server != nil {
		if server.ProtoRoot != "" && !validationSafeProtoRoot(server.ProtoRoot) {
			v.add(IssuePathInvalid, "components.grpc.server.proto_root")
		}
		if server.BufConfig != "" && !validationSafeRelative(server.BufConfig) {
			v.add(IssuePathInvalid, "components.grpc.server.buf_config")
		}
	}
	if generator := v.project.Manifest.Languages.Go.Generators.GRPC; generator != nil &&
		generator.BufGenConfig != "" && !validationSafeRelative(generator.BufGenConfig) {
		v.add(IssuePathInvalid, "languages.go.generators.grpc.buf_gen_config")
	}
	seen := make(map[string]bool, len(grpc.Clients))
	for _, client := range grpc.Clients {
		v.validateGRPCClient(client, seen[client.Name])
		seen[client.Name] = true
	}
}

func (v *semanticValidation) validateGRPCClient(client GRPCClient, duplicate bool) {
	base := "components.grpc.clients." + client.Name
	if !validationName.MatchString(client.Name) || duplicate {
		v.add(IssueGRPCClientInvalid, base)
	}
	source, exists := v.project.Manifest.Sources[client.Source]
	if !exists {
		v.add(IssueSourceNotFound, base+".source")
		return
	}
	if clientContractSelectionInvalid(source, client.Export, client.Path) {
		v.add(IssueGRPCClientInvalid, base)
	}
	if client.Path != "" && !validationSafeRelative(client.Path) {
		v.add(IssuePathInvalid, base+".path")
	}
	if client.ProtoRoot != "" && !validationSafeProtoRoot(client.ProtoRoot) {
		v.add(IssuePathInvalid, base+".proto_root")
	}
	if client.BufGenConfig != "" && !validationSafeRelative(client.BufGenConfig) {
		v.add(IssuePathInvalid, base+".buf_gen_config")
	}
}

func clientContractSelectionInvalid(source Source, exported, selectedPath string) bool {
	if source.Type == SourceDevctl {
		return exported == "" || selectedPath != ""
	}
	return selectedPath == "" || exported != ""
}

func (v *semanticValidation) validateKafka() {
	kafka := v.project.Manifest.Components.Kafka
	if kafka == nil {
		return
	}
	for _, consumer := range kafka.Consumers {
		base := "components.kafka.consumers." + consumer.Name + ".contract"
		v.validateKafkaContract(base, consumer.Contract)
		v.validateKafkaSource(base+".source", consumer.Contract)
	}
	for _, producer := range kafka.Producers {
		base := "components.kafka.producers." + producer.Name + ".contract"
		v.validateKafkaContract(base, producer.Contract)
		v.validateKafkaSource(base+".source", producer.Contract)
	}
}

func (v *semanticValidation) validateKafkaContract(field string, selected KafkaContract) {
	format := valueOrDefault(selected.Format, "raw")
	if format == "raw" {
		if selected.Source != "" || selected.Export != "" || selected.Path != "" ||
			selected.ProtoRoot != "" || selected.Message != "" || selected.Encoding != "" {
			v.add(IssueKafkaContractInvalid, field)
		}
		return
	}
	if format != "json" && format != "proto" || selected.Source == "" {
		v.add(IssueKafkaContractInvalid, field)
		return
	}
	source, exists := v.project.Manifest.Sources[selected.Source]
	if !exists {
		return
	}
	if kafkaSourceSelectionInvalid(source, selected) {
		v.add(IssueKafkaContractInvalid, field)
		return
	}
	if selected.Path != "" && !validationSafeRelative(selected.Path) {
		v.add(IssueKafkaContractInvalid, field)
		return
	}
	if format == "json" {
		if selected.ProtoRoot != "" || selected.Message != "" || selected.Encoding != "" {
			v.add(IssueKafkaContractInvalid, field)
		}
		return
	}
	protoRoot := valueOrDefault(selected.ProtoRoot, filepath.ToSlash(filepath.Dir(selected.Path)))
	if !validationSafeProtoRoot(protoRoot) || selected.Path != "" && !validationPathWithin(protoRoot, selected.Path) ||
		selected.Encoding != "" && selected.Encoding != "binary" && selected.Encoding != "json" {
		v.add(IssueKafkaContractInvalid, field)
	}
}

func kafkaSourceSelectionInvalid(source Source, selected KafkaContract) bool {
	if source.Type == SourceDevctl {
		return selected.Export == "" || selected.Path != ""
	}
	return selected.Path == "" || selected.Export != ""
}

func (v *semanticValidation) validateKafkaSource(field string, selected KafkaContract) {
	if selected.Source == "" {
		return
	}
	if _, exists := v.project.Manifest.Sources[selected.Source]; !exists {
		v.add(IssueSourceNotFound, field)
	}
}

func (v *semanticValidation) validateComponentPolicies() {
	manifest := v.project.Manifest
	if manifest.Components.DB != nil {
		v.validateDB(manifest.Components.DB)
	}
	if manifest.Components.S3 != nil {
		v.validateS3(manifest.Components.S3)
	}
	if manifest.Components.Redis != nil {
		v.validateRedis(manifest.Components.Redis)
	}
}

func (v *semanticValidation) validateDB(database *DB) {
	if len(database.Connections) == 0 {
		v.add(IssueDBConnectionInvalid, "components.db.connections")
		return
	}
	seen := make(map[string]bool, len(database.Connections))
	for _, connection := range database.Connections {
		v.validateDBConnection(connection, seen[connection.Name])
		seen[connection.Name] = true
	}
}

func (v *semanticValidation) validateDBConnection(connection DBConnection, duplicate bool) {
	base := "components.db.connections." + connection.Name
	if !validationName.MatchString(connection.Name) || duplicate {
		v.add(IssueDBConnectionInvalid, base)
	}
	seen := make(map[string]bool, len(connection.Variants))
	for _, variant := range connection.Variants {
		validKind := variant.Kind == "sqlite" || variant.Kind == "postgres" || variant.Kind == "clickhouse"
		if !validationName.MatchString(variant.Name) || seen[variant.Name] || !validKind {
			v.add(IssueDBVariantInvalid, base)
		}
		seen[variant.Name] = true
		if variant.Migrations != nil && !validDBMigrations(variant.Kind, variant.Migrations) {
			v.add(IssueDBMigrationsInvalid, base+".variants."+variant.Name+".migrations")
		}
	}
	if connection.Default == "" && len(connection.Variants) != 1 ||
		connection.Default != "" && !seen[connection.Default] {
		v.add(IssueDBDefaultInvalid, base+".default")
	}
}

func validDBMigrations(kind string, migrations *DBMigrations) bool {
	if migrations == nil {
		return true
	}
	if kind != "sqlite" && kind != "postgres" && kind != "clickhouse" ||
		!validationSafeRelative(migrations.Path) || !validationEnvironmentKey.MatchString(migrations.DatabaseEnv) {
		return false
	}
	if migrations.DatabaseDefault == "" {
		return true
	}
	parsed, err := url.Parse(migrations.DatabaseDefault)
	if err != nil {
		return false
	}
	switch kind {
	case "sqlite":
		return parsed.Scheme == "sqlite"
	case "clickhouse":
		return parsed.Scheme == "clickhouse"
	default:
		return parsed.Scheme == "postgres" || parsed.Scheme == "postgresql"
	}
}

func (v *semanticValidation) validateRedis(redis *Redis) {
	seen := make(map[string]bool, len(redis.Connections))
	for _, connection := range redis.Connections {
		base := "components.redis.connections." + connection.Name
		if !validationName.MatchString(connection.Name) || seen[connection.Name] ||
			!validationEnvironmentKey.MatchString(connection.AddrEnv) {
			v.add(IssueRedisConnectionInvalid, base)
		}
		if connection.AddrDefault != "" && !validRedisAddress(connection.AddrDefault) {
			v.add(IssueRedisAddressInvalid, base+".addr_default")
		}
		seen[connection.Name] = true
	}
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

func (v *semanticValidation) validateS3(storage *S3) {
	connections := make(map[string]struct{}, len(storage.Connections))
	for _, connection := range storage.Connections {
		connections[connection.Name] = struct{}{}
	}
	for _, bucket := range storage.Buckets {
		if _, exists := connections[bucket.Connection]; !exists {
			v.add(IssueS3ConnectionNotFound, "components.s3.buckets."+bucket.Name+".connection")
		}
	}
}

func (v *semanticValidation) validateRuntimeConfig() {
	_, err := NewRuntimeConfigCatalog(v.project.Manifest)
	var conflict *RuntimeConfigConflictError
	if errors.As(err, &conflict) {
		v.add(IssueRuntimeConfigConflict, "env")
	}
}

func (v *semanticValidation) validateOutputPaths() {
	paths := managedOutputPaths(v.project.Manifest)
	for _, candidate := range paths {
		if candidate.value != "" && !validationSafeRelative(candidate.value) {
			v.add(IssuePathInvalid, candidate.field)
		}
	}
	for index := range paths {
		v.validateOutputOverlaps(paths, index)
	}
}

type managedPath struct {
	field string
	value string
}

func managedOutputPaths(manifest Manifest) []managedPath {
	paths := []managedPath{{field: "paths.external_contracts", value: valueOrDefault(manifest.Paths.ExternalContracts, "api/external")}}
	seen := make(map[managedPath]struct{})
	for _, target := range NewTargetCatalog(manifest).All() {
		candidate := managedPath{field: targetOutputField(target), value: target.OutputDir}
		if candidate.field == "" || candidate.value == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		paths = append(paths, candidate)
	}
	return paths
}

func (v *semanticValidation) validateOutputOverlaps(paths []managedPath, leftIndex int) {
	leftCandidate := paths[leftIndex]
	if leftCandidate.value == "" {
		return
	}
	left := filepath.Clean(filepath.FromSlash(leftCandidate.value))
	for _, rightCandidate := range paths[leftIndex+1:] {
		if rightCandidate.value == "" {
			continue
		}
		right := filepath.Clean(filepath.FromSlash(rightCandidate.value))
		if validationPathsOverlap(left, right) {
			v.add(IssuePathOverlap, leftCandidate.field)
		}
	}
}

func targetOutputField(target Target) string {
	switch target.Family {
	case "config":
		return "languages.go.generators.config.out"
	case "http":
		if target.Role == "server" {
			return "languages.go.generators.http.server_out"
		}
		return "languages.go.generators.http.client_out"
	case "grpc":
		return "languages.go.generators.grpc.out"
	case "kafka":
		return "languages.go.generators.kafka.out"
	default:
		return ""
	}
}

func validationSafeRelative(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") {
		return false
	}
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func validationSafeProtoRoot(value string) bool {
	return value == "." || validationSafeRelative(value)
}

func validationPathWithin(root, selected string) bool {
	root = filepath.ToSlash(filepath.Clean(root))
	selected = filepath.ToSlash(filepath.Clean(selected))
	if root == "." {
		return validationSafeRelative(selected)
	}
	return selected == root || strings.HasPrefix(selected, root+"/")
}

func validationPathsOverlap(left, right string) bool {
	separator := string(filepath.Separator)
	return left == right || strings.HasPrefix(left, right+separator) || strings.HasPrefix(right, left+separator)
}

func validationSortedKeys[Value any](values map[string]Value) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
