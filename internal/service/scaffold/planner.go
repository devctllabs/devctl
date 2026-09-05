package scaffold

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

// Artifact is one rendered scaffold file and its replacement policy.
type Artifact struct {
	Path    string
	Mode    fs.FileMode
	Content []byte
	// CreateOnly preserves different existing content on every refresh.
	CreateOnly bool
}

type conflict struct {
	code string
	path string
}

type preflightInspector struct {
	workspace WorkspaceRepository
	root      string
	planned   map[string]Artifact
	conflicts []conflict
}

type preflightRequest struct {
	root      string
	artifacts []Artifact
}

// preflight collects all detectable workspace conflicts before the first scaffold publication.
func preflight(
	ctx context.Context,
	workspace WorkspaceRepository,
	request preflightRequest,
) ([]conflict, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ctx.Err: %w", err)
	}
	planned := make(map[string]Artifact, len(request.artifacts))
	for _, artifact := range request.artifacts {
		planned[canonicalArtifactPath(artifact.Path)] = artifact
	}
	inspector := preflightInspector{workspace: workspace, root: request.root, planned: planned}
	err := workspace.Walk(ctx, request.root, func(path string, entry fs.DirEntry, walkErr error) error {
		return inspector.inspectPath(ctx, path, entry, walkErr)
	})
	if err != nil {
		return nil, fmt.Errorf("workspace.Walk: %w", err)
	}
	return inspector.conflicts, nil
}

// plan returns the complete rendered artifact set in deterministic path order with formatted Go sources.
func plan(m projectdomain.Manifest) ([]Artifact, error) {
	projection := compileScaffoldProjection(m)
	artifacts, err := baseArtifacts(projection)
	if err != nil {
		return nil, fmt.Errorf("baseArtifacts: %w", err)
	}
	server, err := serverArtifacts(projection)
	if err != nil {
		return nil, fmt.Errorf("serverArtifacts: %w", err)
	}
	artifacts = append(artifacts, server...)
	http, err := httpArtifacts(projection.http)
	if err != nil {
		return nil, fmt.Errorf("httpArtifacts: %w", err)
	}
	artifacts = append(artifacts, http...)
	proto, err := protoArtifacts(projection.proto)
	if err != nil {
		return nil, fmt.Errorf("protoArtifacts: %w", err)
	}
	artifacts = append(artifacts, proto...)
	database, err := dbArtifacts(projection)
	if err != nil {
		return nil, fmt.Errorf("dbArtifacts: %w", err)
	}
	artifacts = append(artifacts, database...)
	components, err := componentArtifacts(projection.components)
	if err != nil {
		return nil, fmt.Errorf("componentArtifacts: %w", err)
	}
	artifacts = append(artifacts, components...)
	applyArtifactOwnership(projection, artifacts)
	if err := ensureUniqueArtifactPaths(artifacts); err != nil {
		return nil, fmt.Errorf("ensureUniqueArtifactPaths: %w", err)
	}
	sort.SliceStable(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	if err := formatGoArtifacts(artifacts); err != nil {
		return nil, fmt.Errorf("formatGoArtifacts: %w", err)
	}
	return artifacts, nil
}

func applyArtifactOwnership(projection scaffoldProjection, artifacts []Artifact) {
	for index := range artifacts {
		artifacts[index].CreateOnly = projection.scaffoldSeed(artifacts[index].Path)
	}
}

func componentArtifacts(projection componentProjection) ([]Artifact, error) {
	builders := []func(componentProjection) ([]Artifact, error){
		grpcComponentArtifacts,
		httpClientArtifacts,
		resourceComponentArtifacts,
		kafkaComponentArtifacts,
	}
	var artifacts []Artifact
	for _, build := range builders {
		group, err := build(projection)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, group...)
	}
	return artifacts, nil
}

func grpcComponentArtifacts(projection componentProjection) ([]Artifact, error) {
	if !projection.grpcEnabled {
		return nil, nil
	}
	grpc, err := renderedTemplateArtifact("internal/deps/grpc.gen.go", "grpc.go.gotmpl", nil)
	if err != nil {
		return nil, err
	}
	artifacts := []Artifact{grpc}
	if len(projection.grpcClients) == 0 {
		return artifacts, nil
	}
	clients, err := renderedTemplateArtifact("internal/deps/grpc_clients.gen.go", "grpc_clients.go.gotmpl", projection.grpcClients)
	if err != nil {
		return nil, err
	}
	return append(artifacts, clients), nil
}

func httpClientArtifacts(projection componentProjection) ([]Artifact, error) {
	if len(projection.httpClients) == 0 {
		return nil, nil
	}
	artifact, err := renderedTemplateArtifact("internal/deps/http_clients.gen.go", "http_clients.go.gotmpl", projection.httpClients)
	return artifactSlice(artifact, err)
}

func resourceComponentArtifacts(projection componentProjection) ([]Artifact, error) {
	var artifacts []Artifact
	if projection.redisEnabled {
		artifact, err := renderedTemplateArtifact("internal/deps/redis.gen.go", "redis.go.gotmpl", projection.redisConnections)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	if projection.s3 != nil {
		artifact, err := renderedTemplateArtifact("internal/deps/s3.gen.go", "s3.go.gotmpl", projection.s3)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func kafkaComponentArtifacts(projection componentProjection) ([]Artifact, error) {
	if projection.kafka == nil {
		return nil, nil
	}
	kafka := projection.kafka
	definitions := []struct {
		path     string
		template string
		data     any
	}{
		{"internal/deps/kafka_broker.gen.go", "kafka_broker.go.gotmpl", nil},
		{"internal/deps/kafka_consumers.gen.go", "kafka_consumers.go.gotmpl", struct {
			Module    string
			Consumers []kafkaConsumerTemplateData
		}{kafka.module, kafka.consumers}},
		{"internal/deps/kafka_producers.gen.go", "kafka_producers.go.gotmpl", kafka.producers},
		{filepath.ToSlash(filepath.Join("cmd", kafka.projectName, "internal", "consumer.go")), "consumer.go.gotmpl", struct{ Module string }{kafka.module}},
	}
	artifacts := make([]Artifact, 0, len(definitions)+2*len(kafka.consumers))
	for _, definition := range definitions {
		artifact, err := renderedTemplateArtifact(definition.path, definition.template, definition.data)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	consumerSeeds, err := kafkaConsumerSeedArtifacts(kafka.consumerSeedFacts)
	if err != nil {
		return nil, err
	}
	return append(artifacts, consumerSeeds...), nil
}

func kafkaConsumerSeedArtifacts(facts []kafkaConsumerSeedFact) ([]Artifact, error) {
	artifacts := make([]Artifact, 0, 2*len(facts))
	for _, fact := range facts {
		binding, err := renderedTemplateArtifact(filepath.ToSlash(filepath.Join("internal", "deps", "consumer_"+fact.packageName+".go")), "kafka_consumer_binding.go.gotmpl", fact.data)
		if err != nil {
			return nil, err
		}
		handler, err := renderedTemplateArtifact(filepath.ToSlash(filepath.Join("internal", "transport", "consumerkafka", fact.packageName, "handler.go")), "kafka_handler.go.gotmpl", struct{ Package string }{fact.packageName})
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, binding, handler)
	}
	return artifacts, nil
}

func renderedTemplateArtifact(output, templateName string, data any) (Artifact, error) {
	content, err := executeTemplate(templateName, data)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{Path: output, Mode: 0o644, Content: []byte(content)}, nil
}

func artifactSlice(artifact Artifact, err error) ([]Artifact, error) {
	if err != nil {
		return nil, err
	}
	return []Artifact{artifact}, nil
}

type kafkaConsumerTemplateData struct {
	Name   string
	Topic  string
	Toggle bool
	Format string
	Module string
}

func protoArtifacts(projection protoProjection) ([]Artifact, error) {
	if !projection.enabled {
		return nil, nil
	}
	generated, err := readTemplateAsset("buf-go.gen.yaml")
	if err != nil {
		return nil, fmt.Errorf("readTemplateAsset: %w", err)
	}
	artifacts := make([]Artifact, 0, len(projection.configPaths)+1)
	for _, configPath := range projection.configPaths {
		artifacts = append(artifacts, Artifact{configPath, 0o644, generated, false})
	}
	moduleArtifact, err := grpcModuleArtifact(projection.grpcModule)
	if err != nil {
		return nil, err
	}
	if moduleArtifact != nil {
		artifacts = append(artifacts, *moduleArtifact)
	}
	return artifacts, nil
}

func grpcModuleArtifact(module *grpcModuleProjection) (*Artifact, error) {
	if module == nil {
		return nil, nil
	}
	moduleConfig, err := executeTemplate("buf.yaml.gotmpl", struct{ ProtoRoot string }{ProtoRoot: module.protoRoot})
	if err != nil {
		return nil, fmt.Errorf("executeTemplate: %w", err)
	}
	artifact := Artifact{module.path, 0o644, []byte(moduleConfig), false}
	return &artifact, nil
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func ensureUniqueArtifactPaths(artifacts []Artifact) error {
	seen := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		path := canonicalArtifactPath(artifact.Path)
		if _, exists := seen[path]; exists {
			return fmt.Errorf("duplicate artifact path %q", path)
		}
		seen[path] = struct{}{}
	}
	return nil
}

func canonicalArtifactPath(path string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
}

func baseArtifacts(projection scaffoldProjection) ([]Artifact, error) {
	goMod, err := renderGoMod(projection.goMod)
	if err != nil {
		return nil, fmt.Errorf("renderGoMod: %w", err)
	}
	mainFile, err := renderMain(projection.main)
	if err != nil {
		return nil, fmt.Errorf("renderMain: %w", err)
	}
	configArtifacts, err := runtimeConfigArtifacts(projection.config)
	if err != nil {
		return nil, fmt.Errorf("runtimeConfigArtifacts: %w", err)
	}
	containerFile, err := renderContainer(projection.container)
	if err != nil {
		return nil, fmt.Errorf("renderContainer: %w", err)
	}
	applicationFile, err := renderApplication(projection.application)
	if err != nil {
		return nil, fmt.Errorf("renderApplication: %w", err)
	}
	projectReadme, err := executeTemplate("project-readme.md.gotmpl", struct{ Project string }{Project: projection.projectName})
	if err != nil {
		return nil, fmt.Errorf("executeTemplate project README: %w", err)
	}
	mise, err := renderMise(projection.mise)
	if err != nil {
		return nil, fmt.Errorf("renderMise: %w", err)
	}
	golangci, err := readTemplateAsset("golangci.yml")
	if err != nil {
		return nil, fmt.Errorf("readTemplateAsset: %w", err)
	}
	artifacts := []Artifact{
		{"go.mod", 0o644, []byte(goMod), false},
		{"README.md", 0o644, []byte(projectReadme), false},
		{".mise.toml", 0o644, []byte(mise), false},
		{".golangci.yml", 0o644, golangci, false},
		{filepath.ToSlash(filepath.Join("cmd", projection.projectName, "main.go")), 0o644, []byte(mainFile), false},
		{"internal/deps/container.gen.go", 0o644, []byte(containerFile), false},
		{"internal/deps/application.go", 0o644, []byte(applicationFile), false},
	}
	artifacts = append(artifacts, configArtifacts...)
	return artifacts, nil
}

func serverArtifacts(projection scaffoldProjection) ([]Artifact, error) {
	if !projection.hasServer {
		return nil, nil
	}
	runtimeFile, err := renderRuntime(projection.runtime)
	if err != nil {
		return nil, fmt.Errorf("renderRuntime: %w", err)
	}
	apiFile, err := renderAPI(projection.module)
	if err != nil {
		return nil, fmt.Errorf("renderAPI: %w", err)
	}
	return []Artifact{
		{"internal/deps/runtime.gen.go", 0o644, []byte(runtimeFile), false},
		{filepath.ToSlash(filepath.Join("cmd", projection.projectName, "internal", "api.go")), 0o644, []byte(apiFile), false},
	}, nil
}

func httpArtifacts(projection httpProjection) ([]Artifact, error) {
	var artifacts []Artifact
	if !projection.enabled {
		return artifacts, nil
	}
	for _, target := range projection.targets {
		templateName := "oapi-client.yaml"
		if target.Role == "server" {
			seed, err := readTemplateAsset("openapi.yaml")
			if err != nil {
				return nil, fmt.Errorf("readTemplateAsset: %w", err)
			}
			artifacts = append(artifacts, Artifact{target.Reference.Entrypoint, 0o644, seed, false})
			templateName = "oapi-server.yaml"
		}
		config, err := readTemplateAsset(templateName)
		if err != nil {
			return nil, fmt.Errorf("readTemplateAsset: %w", err)
		}
		artifacts = append(artifacts, Artifact{target.Config, 0o644, config, false})
	}
	return artifacts, nil
}

func formatGoArtifacts(artifacts []Artifact) error {
	for index := range artifacts {
		artifact := &artifacts[index]
		if strings.HasSuffix(artifact.Path, ".gen.go") && !bytes.HasPrefix(artifact.Content, []byte("// Code generated by devctl. DO NOT EDIT.")) {
			artifact.Content = append([]byte("// Code generated by devctl. DO NOT EDIT.\n\n"), artifact.Content...)
		}
		if strings.HasSuffix(artifact.Path, ".go") {
			formatted, formatErr := format.Source(artifact.Content)
			if formatErr != nil {
				return fmt.Errorf("format.Source: %s: %w", artifact.Path, formatErr)
			}
			artifact.Content = formatted
		}
	}
	return nil
}

func dbArtifacts(projection scaffoldProjection) ([]Artifact, error) {
	if len(projection.storages) == 0 {
		return nil, nil
	}
	artifacts := make([]Artifact, 0, len(projection.storages)+1)
	for _, storage := range projection.storages {
		storageFile, err := renderStorage(storage.template)
		if err != nil {
			return nil, fmt.Errorf("renderStorage: %w", err)
		}
		artifacts = append(artifacts, Artifact{storage.path, 0o644, []byte(storageFile), false})
		for _, migrationPath := range storage.migrationPaths {
			artifacts = append(artifacts, Artifact{
				Path: filepath.ToSlash(filepath.Join(migrationPath, ".gitkeep")),
				Mode: 0o644, Content: []byte{}, CreateOnly: false,
			})
		}
	}
	if projection.hasSQLite {
		artifacts = append(artifacts, Artifact{"data/.gitkeep", 0o644, []byte{}, false})
	}
	return artifacts, nil
}

func (i *preflightInspector) inspectPath(ctx context.Context, path string, entry fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	relative := filepath.ToSlash(path)
	if relative == "." {
		return nil
	}
	if strings.HasPrefix(relative, ".git/") {
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	clean := filepath.ToSlash(filepath.Clean(relative))
	if artifact, exists := i.planned[clean]; exists {
		return i.inspectArtifact(ctx, artifact, clean, relative)
	}
	return nil
}

func (i *preflightInspector) inspectArtifact(ctx context.Context, artifact Artifact, clean, relative string) error {
	info, err := i.workspace.Lstat(ctx, i.root, clean)
	if err != nil {
		return fmt.Errorf("workspace.Lstat: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		i.conflicts = append(i.conflicts, conflict{code: "wrong_file_kind", path: relative})
		return nil
	}
	_, err = i.workspace.ReadBytes(ctx, i.root, clean)
	if err != nil {
		return fmt.Errorf("workspace.ReadBytes: %w", err)
	}
	return nil
}
