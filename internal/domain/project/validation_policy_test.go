package project_test

import (
	"testing"

	"github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestValidateContextFreePolicyGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*project.Manifest)
		expected []project.Issue
	}{
		{
			name: "sources",
			mutate: func(manifest *project.Manifest) {
				manifest.Sources = map[string]project.Source{
					"Bad":      {Type: project.SourceLocal, Path: "contracts"},
					"devctl":   {Type: project.SourceDevctl, Repo: "example/contracts", Ref: "v1", Path: "unexpected"},
					"insecure": {Type: project.SourceURL, URL: "http://example.test/openapi.yaml"},
					"unknown":  {Type: project.SourceType("ftp")},
				}
			},
			expected: []project.Issue{
				{Code: project.IssueSourceNameInvalid, Path: projectManifestPath, Field: "sources.Bad"},
				{Code: project.IssueSourceInvalid, Path: projectManifestPath, Field: "sources.devctl"},
				{Code: project.IssueSourceInvalid, Path: projectManifestPath, Field: "sources.insecure"},
				{Code: project.IssueSourceInsecure, Path: projectManifestPath, Field: "sources.insecure"},
				{Code: project.IssueSourceInvalid, Path: projectManifestPath, Field: "sources.unknown"},
				{Code: project.IssueSourceTypeUnsupported, Path: projectManifestPath, Field: "sources.unknown"},
			},
		},
		{
			name: "exports are checked in name order",
			mutate: func(manifest *project.Manifest) {
				manifest.Exports = map[string]project.Export{
					"zeta":  {Kind: "openapi", Path: "api/zeta.yaml"},
					"alpha": {Kind: "proto", Path: "api/alpha.proto"},
				}
			},
			expected: []project.Issue{
				{Code: project.IssueExportInvalid, Path: projectManifestPath, Field: "exports.alpha"},
				{Code: project.IssueExportInvalid, Path: projectManifestPath, Field: "exports.zeta"},
			},
		},
		{
			name: "http",
			mutate: func(manifest *project.Manifest) {
				manifest.Sources = map[string]project.Source{
					"local": {Type: project.SourceLocal, Path: "contracts"},
				}
				manifest.Components.HTTP = &project.HTTP{
					Server: &project.HTTPServer{OpenAPI: "../openapi.yaml"},
					Clients: []project.HTTPClient{
						{Name: "Bad", Source: "missing"},
						{Name: "local", Source: "local"},
						{Name: "local", Source: "local", Path: "openapi.yaml"},
					},
				}
			},
			expected: []project.Issue{
				{Code: project.IssuePathInvalid, Path: projectManifestPath, Field: "components.http.server.openapi"},
				{Code: project.IssueHTTPClientInvalid, Path: projectManifestPath, Field: "components.http.clients.Bad"},
				{Code: project.IssueSourceNotFound, Path: projectManifestPath, Field: "components.http.clients.Bad.source"},
				{Code: project.IssueHTTPClientInvalid, Path: projectManifestPath, Field: "components.http.clients.local"},
				{Code: project.IssueHTTPClientInvalid, Path: projectManifestPath, Field: "components.http.clients.local"},
			},
		},
		{
			name: "grpc",
			mutate: func(manifest *project.Manifest) {
				manifest.Sources = map[string]project.Source{
					"contracts": {Type: project.SourceLocal, Path: "contracts"},
				}
				manifest.Components.GRPC = &project.GRPC{
					Server: &project.GRPCServer{ProtoRoot: "../proto", BufConfig: "/buf.yaml"},
					Clients: []project.GRPCClient{{
						Name: "client", Source: "contracts", Path: "../client.proto", ProtoRoot: "../proto", BufGenConfig: "/gen.yaml",
					}},
				}
				manifest.Languages.Go.Generators.GRPC = &project.GRPCGenerator{BufGenConfig: "../grpc.gen.yaml"}
			},
			expected: []project.Issue{
				{Code: project.IssuePathInvalid, Path: projectManifestPath, Field: "components.grpc.server.proto_root"},
				{Code: project.IssuePathInvalid, Path: projectManifestPath, Field: "components.grpc.server.buf_config"},
				{Code: project.IssuePathInvalid, Path: projectManifestPath, Field: "languages.go.generators.grpc.buf_gen_config"},
				{Code: project.IssuePathInvalid, Path: projectManifestPath, Field: "components.grpc.clients.client.path"},
				{Code: project.IssuePathInvalid, Path: projectManifestPath, Field: "components.grpc.clients.client.proto_root"},
				{Code: project.IssuePathInvalid, Path: projectManifestPath, Field: "components.grpc.clients.client.buf_gen_config"},
			},
		},
		{
			name: "kafka",
			mutate: func(manifest *project.Manifest) {
				manifest.Sources = map[string]project.Source{
					"contracts": {Type: project.SourceLocal, Path: "contracts"},
				}
				manifest.Components.Kafka = &project.Kafka{
					Consumers: []project.KafkaConsumer{{Name: "raw", Contract: project.KafkaContract{Source: "contracts"}}},
					Producers: []project.KafkaProducer{{Name: "proto", Contract: project.KafkaContract{
						Source: "missing", Path: "event.proto", Format: "proto",
					}}},
				}
			},
			expected: []project.Issue{
				{Code: project.IssueKafkaContractInvalid, Path: projectManifestPath, Field: "components.kafka.consumers.raw.contract"},
				{Code: project.IssueSourceNotFound, Path: projectManifestPath, Field: "components.kafka.producers.proto.contract.source"},
			},
		},
		{
			name: "database redis and s3",
			mutate: func(manifest *project.Manifest) {
				manifest.Components.DB = &project.DB{Connections: []project.DBConnection{{
					Name: "Bad", Variants: []project.DBVariant{{Name: "bad", Kind: "oracle"}},
				}}}
				manifest.Components.S3 = &project.S3{Buckets: []project.S3Bucket{{Name: "media", Connection: "missing"}}}
				manifest.Components.Redis = &project.Redis{Connections: []project.RedisConnection{{
					Name: "cache", AddrEnv: "bad-env", AddrDefault: "localhost",
				}}}
			},
			expected: []project.Issue{
				{Code: project.IssueDBConnectionInvalid, Path: projectManifestPath, Field: "components.db.connections.Bad"},
				{Code: project.IssueDBVariantInvalid, Path: projectManifestPath, Field: "components.db.connections.Bad"},
				{Code: project.IssueS3ConnectionNotFound, Path: projectManifestPath, Field: "components.s3.buckets.media.connection"},
				{Code: project.IssueRedisConnectionInvalid, Path: projectManifestPath, Field: "components.redis.connections.cache"},
				{Code: project.IssueRedisAddressInvalid, Path: projectManifestPath, Field: "components.redis.connections.cache.addr_default"},
			},
		},
		{
			name: "runtime config conflict",
			mutate: func(manifest *project.Manifest) {
				manifest.Env.Custom = []project.EnvGroup{
					{Group: "service", Vars: []project.EnvVar{{Key: "MODE", Type: "string"}}},
					{Group: "worker", Vars: []project.EnvVar{{Key: "MODE", Type: "bool"}}},
				}
			},
			expected: []project.Issue{{Code: project.IssueRuntimeConfigConflict, Path: projectManifestPath, Field: "env"}},
		},
		{
			name: "managed output overlap",
			mutate: func(manifest *project.Manifest) {
				manifest.Paths.ExternalContracts = "gen"
				manifest.Languages.Go.Generators.Config = &project.ConfigGenerator{Out: "gen/config"}
			},
			expected: []project.Issue{{Code: project.IssuePathOverlap, Path: projectManifestPath, Field: "paths.external_contracts"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manifest := validSemanticManifest()
			test.mutate(&manifest)

			issues := project.Validate(project.Project{ManifestPath: projectManifestPath, Manifest: manifest})

			require.Equal(t, test.expected, issues)
		})
	}
}

const projectManifestPath = "/project/devctl.yaml"

func validSemanticManifest() project.Manifest {
	return project.Manifest{
		Version:   1,
		Project:   project.Identity{Name: "example", Language: "go"},
		Languages: project.Languages{Go: project.GoLanguage{Module: "example.test/example"}},
	}
}
