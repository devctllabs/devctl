package project

import (
	"path"
	"sort"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/contract"
)

// TargetOperation identifies a workflow that can address a Target.
type TargetOperation uint8

const (
	TargetOperationSync TargetOperation = iota + 1
	TargetOperationLint
	TargetOperationGenerate
)

// Target contains the stable effective facts shared by target workflows.
type Target struct {
	ID     string
	Family string
	Role   string
	Name   string
	Format string

	SourceName  string
	Source      Source
	SourceFound bool
	Reference   contract.Reference
	Location    contract.Location

	Input      string
	Paths      []string
	Config     string
	OutputDir  string
	OutputFile string
}

// TargetCatalog is an immutable effective Target projection of one Manifest.
type TargetCatalog struct {
	entries []targetCatalogEntry
}

type targetCatalogEntry struct {
	target     Target
	operations targetOperations
}

type targetOperations uint8

const (
	targetSync targetOperations = 1 << iota
	targetLint
	targetGenerate
)

// NewTargetCatalog applies effective defaults and returns Targets sorted by ID.
// It is total: malformed references remain visible for validation instead of returning an error.
func NewTargetCatalog(manifest Manifest) TargetCatalog {
	entries := targetEntries(manifest)
	sort.Slice(entries, func(i, j int) bool { return entries[i].target.ID < entries[j].target.ID })
	return TargetCatalog{entries: entries}
}

// All returns every configured Target as a defensive copy in stable ID order.
func (c TargetCatalog) All() []Target {
	return copyCatalogTargets(c.entries)
}

// Select returns CLI-addressable Targets for operation, family, and id in stable ID order.
// Empty family or id values do not constrain that dimension; an unsupported selection is empty.
func (c TargetCatalog) Select(operation TargetOperation, family, id string) []Target {
	mask := targetOperationMask(operation)
	if mask == 0 {
		return nil
	}
	entries := make([]targetCatalogEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		if entry.operations&mask == 0 || family != "" && entry.target.Family != family || id != "" && entry.target.ID != id {
			continue
		}
		entries = append(entries, entry)
	}
	return copyCatalogTargets(entries)
}

func targetEntries(manifest Manifest) []targetCatalogEntry {
	var entries []targetCatalogEntry
	if generator := manifest.Languages.Go.Generators.Config; generator != nil || manifest.Project.Language == "go" {
		outputDir := "gen/config"
		if generator != nil {
			outputDir = valueOrDefault(generator.Out, outputDir)
		}
		entries = append(entries, targetCatalogEntry{
			target: Target{
				ID: "config", Family: "config", Format: "go",
				OutputDir: outputDir, OutputFile: "config.gen.go",
			},
			operations: targetGenerate,
		})
	}
	entries = append(entries, httpTargetEntries(manifest)...)
	entries = append(entries, grpcTargetEntries(manifest)...)
	entries = append(entries, kafkaTargetEntries(manifest)...)
	return entries
}

func httpTargetEntries(manifest Manifest) []targetCatalogEntry {
	http := manifest.Components.HTTP
	if http == nil {
		return nil
	}
	generator := manifest.Languages.Go.Generators.HTTP
	var entries []targetCatalogEntry
	if server := http.Server; server != nil {
		entrypoint := valueOrDefault(server.OpenAPI, "api/openapi/swagger.yaml")
		entries = append(entries, targetCatalogEntry{
			target: Target{
				ID: "http-server", Family: "http", Role: "server", Format: "openapi",
				Reference: contract.Reference{Entrypoint: entrypoint},
				Location:  contract.Location{RelativePath: entrypoint, Entrypoint: entrypoint, Local: true},
				Input:     entrypoint, Config: httpServerConfig(generator),
				OutputDir: httpServerOutput(generator), OutputFile: "server.gen.go",
			},
			operations: targetLint | targetGenerate,
		})
	}
	for _, client := range http.Clients {
		source, found := manifest.Sources[client.Source]
		location := targetContractLocation(manifest, targetLocationRequest{
			source: source, sourceFound: found, entrypoint: client.Path,
			externalSuffix: path.Join("http", "client", client.Name),
		})
		input := location.RelativePath
		if location.Local {
			input = path.Join(input, client.Path)
		}
		operations := targetLint | targetGenerate
		if found {
			operations |= targetSync
		}
		entries = append(entries, targetCatalogEntry{
			target: Target{
				ID: "http-client:" + client.Name, Family: "http", Role: "client", Name: client.Name, Format: "openapi",
				SourceName: client.Source, Source: source, SourceFound: found,
				Reference: contract.Reference{Entrypoint: client.Path, Export: client.Export}, Location: location,
				Input: input, Config: valueOrDefault(client.OAPIConfig, "tools/oapi/clients."+client.Name+".yaml"),
				OutputDir: path.Join(httpClientOutput(generator), client.Name), OutputFile: "client.gen.go",
			},
			operations: operations,
		})
	}
	return entries
}

func grpcTargetEntries(manifest Manifest) []targetCatalogEntry {
	grpc := manifest.Components.GRPC
	if grpc == nil {
		return nil
	}
	generator := manifest.Languages.Go.Generators.GRPC
	root := grpcOutput(generator)
	var entries []targetCatalogEntry
	if server := grpc.Server; server != nil {
		entries = append(entries, targetCatalogEntry{
			target: Target{
				ID: "grpc-server", Family: "grpc", Role: "server", Format: "proto",
				Reference: contract.Reference{Format: "proto", ProtoRoot: valueOrDefault(server.ProtoRoot, "api/proto/grpc")},
				Input:     valueOrDefault(server.ProtoRoot, "api/proto/grpc"), Config: grpcConfig(generator, ""),
				OutputDir: path.Join(root, "server"),
			},
			operations: targetLint | targetGenerate,
		})
	}
	for _, client := range grpc.Clients {
		source, found := manifest.Sources[client.Source]
		protoRoot := valueOrDefault(client.ProtoRoot, client.Path)
		location := targetContractLocation(manifest, targetLocationRequest{
			source: source, sourceFound: found, entrypoint: client.Path,
			externalSuffix: path.Join("grpc", "client", client.Name),
		})
		inputRoot := location.RelativePath
		if protoRoot != "." {
			inputRoot = path.Join(inputRoot, protoRoot)
		}
		operations := targetLint | targetGenerate
		if found {
			operations |= targetSync
		}
		entries = append(entries, targetCatalogEntry{
			target: Target{
				ID: "grpc-client:" + client.Name, Family: "grpc", Role: "client", Name: client.Name, Format: "proto",
				SourceName: client.Source, Source: source, SourceFound: found,
				Reference: contract.Reference{Entrypoint: client.Path, Export: client.Export, Format: "proto", ProtoRoot: client.ProtoRoot},
				Location:  location, Input: inputRoot, Paths: selectedProtoPaths(client.Path, protoRoot),
				Config: grpcConfig(generator, client.BufGenConfig), OutputDir: path.Join(root, "client", client.Name),
			},
			operations: operations,
		})
	}
	return entries
}

func kafkaTargetEntries(manifest Manifest) []targetCatalogEntry {
	kafka := manifest.Components.Kafka
	if kafka == nil {
		return nil
	}
	entries := make([]targetCatalogEntry, 0, len(kafka.Consumers)+len(kafka.Producers))
	for _, consumer := range kafka.Consumers {
		entries = append(entries, kafkaTargetEntry(manifest, kafkaTargetSelection{
			role: "consumer", name: consumer.Name, topic: consumer.Topic, contract: consumer.Contract,
		}))
	}
	for _, producer := range kafka.Producers {
		entries = append(entries, kafkaTargetEntry(manifest, kafkaTargetSelection{
			role: "producer", name: producer.Name, topic: producer.Topic, contract: producer.Contract,
		}))
	}
	return entries
}

type kafkaTargetSelection struct {
	role     string
	name     string
	topic    string
	contract KafkaContract
}

func kafkaTargetEntry(manifest Manifest, selection kafkaTargetSelection) targetCatalogEntry {
	role, name, topic, selected := selection.role, selection.name, selection.topic, selection.contract
	format := valueOrDefault(selected.Format, "raw")
	target := Target{
		ID: "kafka-" + role + ":" + name, Family: "kafka", Role: role, Name: name, Format: format,
		Reference: contract.Reference{
			Entrypoint: selected.Path, Export: selected.Export, Format: format,
			ProtoRoot: selected.ProtoRoot, Topic: topic,
		},
	}
	operations := targetLint | targetGenerate
	if format == "raw" {
		return targetCatalogEntry{target: target, operations: operations}
	}
	source, found := manifest.Sources[selected.Source]
	target.SourceName, target.Source, target.SourceFound = selected.Source, source, found
	target.Location = targetContractLocation(manifest, targetLocationRequest{
		source: source, sourceFound: found, entrypoint: selected.Path,
		externalSuffix: path.Join("kafka", role, name),
	})
	target.OutputDir = path.Join(kafkaOutput(manifest.Languages.Go.Generators.Kafka), role, name)
	if format == "json" {
		target.Input = path.Join(target.Location.RelativePath, selected.Path)
		target.OutputFile = "schema.gen.go"
	} else {
		protoRoot := valueOrDefault(selected.ProtoRoot, path.Dir(selected.Path))
		target.Input = path.Join(target.Location.RelativePath, protoRoot)
		target.Paths = selectedProtoPaths(selected.Path, protoRoot)
		target.Config = kafkaConfig(manifest.Languages.Go.Generators.Kafka)
	}
	if found {
		operations |= targetSync
	}
	return targetCatalogEntry{target: target, operations: operations}
}

type targetLocationRequest struct {
	source         Source
	sourceFound    bool
	entrypoint     string
	externalSuffix string
}

func targetContractLocation(manifest Manifest, request targetLocationRequest) contract.Location {
	if request.sourceFound && request.source.Type == SourceLocal {
		return contract.Location{RelativePath: request.source.Path, Entrypoint: request.entrypoint, Local: true}
	}
	return contract.Location{
		RelativePath: path.Join(externalContractsRoot(manifest), request.externalSuffix),
		Entrypoint:   request.entrypoint,
	}
}

func selectedProtoPaths(entrypoint, protoRoot string) []string {
	selected := strings.TrimPrefix(path.Clean(entrypoint), path.Clean(protoRoot)+"/")
	if selected == "." {
		return nil
	}
	return []string{selected}
}

func targetOperationMask(operation TargetOperation) targetOperations {
	switch operation {
	case TargetOperationSync:
		return targetSync
	case TargetOperationLint:
		return targetLint
	case TargetOperationGenerate:
		return targetGenerate
	default:
		return 0
	}
}

func copyCatalogTargets(entries []targetCatalogEntry) []Target {
	targets := make([]Target, len(entries))
	for index, entry := range entries {
		targets[index] = entry.target
		targets[index].Paths = append([]string(nil), entry.target.Paths...)
	}
	return targets
}

// SnapshotExpectation binds a Devctl-sourced Target to committed Snapshot Metadata.
func (target Target) SnapshotExpectation() contract.MetadataExpectation {
	expected := contract.MetadataExpectation{Kind: target.Family, Format: target.Format}
	if target.Family == "kafka" {
		expected.Topic = target.Reference.Topic
	}
	return expected
}

// WithSnapshot resolves a Target's concrete input from validated committed metadata.
func (target Target) WithSnapshot(snapshot contract.Snapshot) Target {
	root := target.Location.RelativePath
	switch {
	case target.Family == "grpc":
		target.Input = path.Join(root, snapshot.ModuleRoot)
		target.Paths = nil
	case target.Family == "kafka" && target.Format == "proto":
		target.Input = path.Join(root, snapshot.ModuleRoot)
		target.Paths = selectedProtoPaths(snapshot.Entrypoint, snapshot.ModuleRoot)
		target.Location.Entrypoint = snapshot.Entrypoint
	case target.Family == "kafka" && target.Format == "json":
		target.Input = path.Join(root, snapshot.Entrypoint)
		target.Location.Entrypoint = snapshot.Entrypoint
	}
	return target
}

func valueOrDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func externalContractsRoot(manifest Manifest) string {
	return valueOrDefault(manifest.Paths.ExternalContracts, "api/external")
}

func httpServerConfig(generator *HTTPGenerator) string {
	if generator == nil {
		return "tools/oapi/server.yaml"
	}
	return valueOrDefault(generator.OAPIConfig, "tools/oapi/server.yaml")
}

func httpServerOutput(generator *HTTPGenerator) string {
	if generator == nil {
		return "gen/serverhttp"
	}
	return valueOrDefault(generator.ServerOut, "gen/serverhttp")
}

func httpClientOutput(generator *HTTPGenerator) string {
	if generator == nil {
		return "gen/clienthttp"
	}
	return valueOrDefault(generator.ClientOut, "gen/clienthttp")
}

func grpcConfig(generator *GRPCGenerator, selected string) string {
	if selected != "" {
		return selected
	}
	if generator != nil && generator.BufGenConfig != "" {
		return generator.BufGenConfig
	}
	return "tools/buf/grpc.gen.yaml"
}

func grpcOutput(generator *GRPCGenerator) string {
	if generator == nil {
		return "gen/grpc"
	}
	return valueOrDefault(generator.Out, "gen/grpc")
}

func kafkaConfig(generator *KafkaGenerator) string {
	if generator != nil && generator.BufGenConfig != "" {
		return generator.BufGenConfig
	}
	return "tools/buf/kafka.gen.yaml"
}

func kafkaOutput(generator *KafkaGenerator) string {
	if generator == nil {
		return "gen/kafka"
	}
	return valueOrDefault(generator.Out, "gen/kafka")
}
