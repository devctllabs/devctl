package project_test

import (
	"testing"

	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/failure"
	"github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestTargetCatalogBuildsStableFacts(t *testing.T) {
	t.Parallel()

	localSource := project.Source{Type: project.SourceLocal, Path: "api/contracts"}
	remoteSource := project.Source{Type: project.SourceGit, Repo: "acme/contracts", Ref: "v1"}
	manifest := project.Manifest{
		Paths: project.ManifestPaths{ExternalContracts: "contracts/external"},
		Sources: map[string]project.Source{
			"local":  localSource,
			"remote": remoteSource,
		},
		Components: project.Components{
			HTTP: &project.HTTP{
				Server: &project.HTTPServer{},
				Clients: []project.HTTPClient{{
					Name: "catalog", Source: "remote", Path: "openapi/catalog.yaml",
				}},
			},
			GRPC: &project.GRPC{Clients: []project.GRPCClient{{
				Name: "billing", Source: "local", Path: "proto/acme/billing/v1/service.proto",
				ProtoRoot: "proto", BufGenConfig: "tools/buf/billing.gen.yaml",
			}}},
			Kafka: &project.Kafka{
				Consumers: []project.KafkaConsumer{{
					Name: "audit", Topic: "audit.events.v1", Contract: project.KafkaContract{Format: "raw"},
				}},
				Producers: []project.KafkaProducer{{
					Name: "invoice", Topic: "invoice.events.v1", Contract: project.KafkaContract{
						Source: "remote", Path: "proto/acme/invoice/v1/event.proto", Format: "proto",
						ProtoRoot: "proto", Message: "acme.invoice.v1.Event", Encoding: "binary",
					},
				}},
			},
		},
		Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{
			Config: &project.ConfigGenerator{Out: "generated/config"},
			HTTP:   &project.HTTPGenerator{ServerOut: "generated/http/server", ClientOut: "generated/http/client"},
			GRPC:   &project.GRPCGenerator{Out: "generated/grpc"},
			Kafka:  &project.KafkaGenerator{Out: "generated/kafka", BufGenConfig: "tools/buf/kafka.custom.yaml"},
		}}},
	}

	targets := project.NewTargetCatalog(manifest).All()

	require.Equal(t, []project.Target{
		{
			ID: "config", Family: "config", Format: "go",
			OutputDir: "generated/config", OutputFile: "config.gen.go",
		},
		{
			ID: "grpc-client:billing", Family: "grpc", Role: "client", Name: "billing", Format: "proto",
			SourceName: "local", Source: localSource, SourceFound: true,
			Reference: contract.Reference{Entrypoint: "proto/acme/billing/v1/service.proto", Format: "proto", ProtoRoot: "proto"},
			Location:  contract.Location{RelativePath: "api/contracts", Entrypoint: "proto/acme/billing/v1/service.proto", Local: true},
			Input:     "api/contracts/proto", Paths: []string{"acme/billing/v1/service.proto"},
			Config: "tools/buf/billing.gen.yaml", OutputDir: "generated/grpc/client/billing",
		},
		{
			ID: "http-client:catalog", Family: "http", Role: "client", Name: "catalog", Format: "openapi",
			SourceName: "remote", Source: remoteSource, SourceFound: true,
			Reference: contract.Reference{Entrypoint: "openapi/catalog.yaml"},
			Location:  contract.Location{RelativePath: "contracts/external/http/client/catalog", Entrypoint: "openapi/catalog.yaml"},
			Input:     "contracts/external/http/client/catalog", Config: "tools/oapi/clients.catalog.yaml",
			OutputDir: "generated/http/client/catalog", OutputFile: "client.gen.go",
		},
		{
			ID: "http-server", Family: "http", Role: "server", Format: "openapi",
			Reference: contract.Reference{Entrypoint: "api/openapi/swagger.yaml"},
			Location:  contract.Location{RelativePath: "api/openapi/swagger.yaml", Entrypoint: "api/openapi/swagger.yaml", Local: true},
			Input:     "api/openapi/swagger.yaml", Config: "tools/oapi/server.yaml",
			OutputDir: "generated/http/server", OutputFile: "server.gen.go",
		},
		{
			ID: "kafka-consumer:audit", Family: "kafka", Role: "consumer", Name: "audit", Format: "raw",
			Reference: contract.Reference{Format: "raw", Topic: "audit.events.v1"},
		},
		{
			ID: "kafka-producer:invoice", Family: "kafka", Role: "producer", Name: "invoice", Format: "proto",
			SourceName: "remote", Source: remoteSource, SourceFound: true,
			Reference: contract.Reference{
				Entrypoint: "proto/acme/invoice/v1/event.proto", Format: "proto", ProtoRoot: "proto", Topic: "invoice.events.v1",
			},
			Location: contract.Location{RelativePath: "contracts/external/kafka/producer/invoice", Entrypoint: "proto/acme/invoice/v1/event.proto"},
			Input:    "contracts/external/kafka/producer/invoice/proto", Paths: []string{"acme/invoice/v1/event.proto"},
			Config: "tools/buf/kafka.custom.yaml", OutputDir: "generated/kafka/producer/invoice",
		},
	}, targets)
}

func TestTargetCatalogSelectsCLIAddressableTargets(t *testing.T) {
	t.Parallel()

	manifest := project.Manifest{
		Sources: map[string]project.Source{
			"local":  {Type: project.SourceLocal, Path: "api/contracts"},
			"remote": {Type: project.SourceURL, URL: "https://example.test/openapi.yaml"},
		},
		Components: project.Components{
			HTTP: &project.HTTP{
				Server: &project.HTTPServer{},
				Clients: []project.HTTPClient{
					{Name: "local", Source: "local", Path: "openapi.yaml"},
					{Name: "remote", Source: "remote", Path: "openapi.yaml"},
				},
			},
			Kafka: &project.Kafka{
				Consumers: []project.KafkaConsumer{{Name: "raw", Contract: project.KafkaContract{Format: "raw"}}},
				Producers: []project.KafkaProducer{{
					Name: "schema", Contract: project.KafkaContract{Source: "local", Path: "schema.json", Format: "json"},
				}},
			},
		},
		Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{Config: &project.ConfigGenerator{}}}},
	}
	catalog := project.NewTargetCatalog(manifest)

	require.Equal(t,
		[]string{"http-client:local", "http-client:remote", "kafka-producer:schema"},
		targetIDs(catalog.Select(project.TargetOperationSync, "", "")),
	)
	require.Equal(t,
		[]string{"http-client:local", "http-client:remote", "http-server", "kafka-consumer:raw", "kafka-producer:schema"},
		targetIDs(catalog.Select(project.TargetOperationLint, "", "")),
	)
	require.Equal(t,
		[]string{"config", "http-client:local", "http-client:remote", "http-server", "kafka-consumer:raw", "kafka-producer:schema"},
		targetIDs(catalog.Select(project.TargetOperationGenerate, "", "")),
	)
	require.Equal(t,
		[]string{"http-client:local"},
		targetIDs(catalog.Select(project.TargetOperationSync, "http", "http-client:local")),
	)
	require.Empty(t, catalog.Select(project.TargetOperationSync, "kafka", "kafka-consumer:raw"))
	require.Empty(t, catalog.Select(project.TargetOperationGenerate, "grpc", "http-server"))
}

func TestTargetCatalogDefaultsGoConfigTarget(t *testing.T) {
	t.Parallel()

	manifest := project.Manifest{
		Project:   project.Identity{Language: "go"},
		Languages: project.Languages{Go: project.GoLanguage{Module: "example.test/sample"}},
	}

	targets := project.NewTargetCatalog(manifest).Select(project.TargetOperationGenerate, "config", "")

	require.Equal(t, []project.Target{{
		ID: "config", Family: "config", Format: "go",
		OutputDir: "gen/config", OutputFile: "config.gen.go",
	}}, targets)
}

func TestTargetCatalogIsTotalAndReturnsDefensiveCopies(t *testing.T) {
	t.Parallel()

	manifest := project.Manifest{Components: project.Components{GRPC: &project.GRPC{Clients: []project.GRPCClient{{
		Name: "missing", Source: "unknown", Path: "proto/missing.proto", ProtoRoot: "proto",
	}}}}}
	catalog := project.NewTargetCatalog(manifest)

	targets := catalog.All()
	require.Len(t, targets, 1)
	require.Equal(t, "unknown", targets[0].SourceName)
	require.False(t, targets[0].SourceFound)
	require.Equal(t, "api/external/grpc/client/missing/proto", targets[0].Input)

	targets[0].ID = "changed"
	targets[0].Paths[0] = "changed.proto"
	require.Equal(t, "grpc-client:missing", catalog.All()[0].ID)
	require.Equal(t, []string{"missing.proto"}, catalog.All()[0].Paths)
}

func TestTargetCatalogAppliesOneWorkflowSelectionContract(t *testing.T) {
	t.Parallel()

	manifest := project.Manifest{
		Project: project.Identity{Language: "go"},
		Sources: map[string]project.Source{
			"local": {Type: project.SourceLocal, Path: "api/contracts"},
		},
		Components: project.Components{
			HTTP: &project.HTTP{Clients: []project.HTTPClient{{Name: "local", Source: "local", Path: "openapi.yaml"}}},
			Kafka: &project.Kafka{Consumers: []project.KafkaConsumer{{
				Name: "raw", Topic: "sample.events.raw.v1", Contract: project.KafkaContract{Format: "raw"},
			}}},
		},
	}
	catalog := project.NewTargetCatalog(manifest)

	tests := []struct {
		name      string
		operation project.TargetOperation
		family    string
		id        string
		ids       []string
		category  failure.Category
	}{
		{name: "known empty family", operation: project.TargetOperationSync, family: "grpc", ids: []string{}},
		{name: "known empty lint family", operation: project.TargetOperationLint, family: "config", ids: []string{}},
		{name: "local sync no-op is supported", operation: project.TargetOperationSync, family: "http", id: "http-client:local", ids: []string{"http-client:local"}},
		{name: "unknown family", operation: project.TargetOperationSync, family: "other", category: failure.InvalidInput},
		{name: "unknown lint family", operation: project.TargetOperationLint, family: "other", category: failure.InvalidInput},
		{name: "unknown target", operation: project.TargetOperationGenerate, id: "grpc-client:missing", category: failure.NotFound},
		{name: "existing target unsupported by operation", operation: project.TargetOperationSync, id: "config", category: failure.Unsupported},
		{name: "existing target unsupported by lint", operation: project.TargetOperationLint, id: "config", category: failure.Unsupported},
		{name: "raw Kafka does not support sync", operation: project.TargetOperationSync, id: "kafka-consumer:raw", category: failure.Unsupported},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			targets, err := catalog.Resolve(test.operation, test.family, test.id)

			if test.category != "" {
				require.Equal(t, test.category, failure.CategoryOf(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.ids, targetIDs(targets))
		})
	}
}

func targetIDs(targets []project.Target) []string {
	ids := make([]string, len(targets))
	for index, target := range targets {
		ids[index] = target.ID
	}
	return ids
}
