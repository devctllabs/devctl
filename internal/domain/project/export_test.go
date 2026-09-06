package project_test

import (
	"testing"

	"github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestManifestExportMatchesEffectiveSurface(t *testing.T) {
	t.Parallel()

	manifest := project.Manifest{Components: project.Components{
		HTTP: &project.HTTP{Server: &project.HTTPServer{OpenAPI: "api/openapi.yaml"}},
		GRPC: &project.GRPC{Server: &project.GRPCServer{ProtoRoot: "api/proto/grpc"}},
		Kafka: &project.Kafka{Producers: []project.KafkaProducer{{
			Name: "audit", Topic: "audit.events", Contract: project.KafkaContract{Format: "json"},
		}}},
	}}
	tests := []struct {
		name     string
		exported project.Export
		expected bool
	}{
		{name: "OpenAPI", exported: project.Export{Kind: "openapi", Path: "api/openapi.yaml"}, expected: true},
		{name: "OpenAPI mismatch", exported: project.Export{Kind: "openapi", Path: "api/other.yaml"}},
		{name: "gRPC", exported: project.Export{Kind: "grpc", Path: "api/proto/grpc"}, expected: true},
		{name: "gRPC mismatch", exported: project.Export{Kind: "grpc", Path: "api/proto/other"}},
		{name: "Kafka", exported: project.Export{Kind: "kafka", Producer: "audit"}, expected: true},
		{name: "Kafka missing producer", exported: project.Export{Kind: "kafka", Producer: "missing"}},
		{name: "unknown kind", exported: project.Export{Kind: "graphql", Path: "api/schema.graphql"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.expected, manifest.ExportMatchesSurface(test.exported))
		})
	}
}
