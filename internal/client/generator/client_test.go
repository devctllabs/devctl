package generator_test

import (
	"context"
	"errors"
	"testing"

	generatorclient "github.com/devctllabs/devctl/internal/client/generator"
	"github.com/devctllabs/devctl/internal/client/generator/mocks"
	"github.com/devctllabs/devctl/internal/domain/artifact"
	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestClientRoutesSupportedTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		target   projectdomain.Target
		selected func(*mocks.MockAdapter, *mocks.MockAdapter, *mocks.MockAdapter) *mocks.MockAdapter
	}{
		{
			name:     "http to OpenAPI",
			target:   projectdomain.Target{ID: "http-client:billing", Family: "http", Format: "openapi"},
			selected: func(openAPI, _, _ *mocks.MockAdapter) *mocks.MockAdapter { return openAPI },
		},
		{
			name:     "gRPC to Proto",
			target:   projectdomain.Target{ID: "grpc-server", Family: "grpc", Format: "proto"},
			selected: func(_, proto, _ *mocks.MockAdapter) *mocks.MockAdapter { return proto },
		},
		{
			name:     "Kafka Proto to Proto",
			target:   projectdomain.Target{ID: "kafka-producer:audit", Family: "kafka", Format: "proto"},
			selected: func(_, proto, _ *mocks.MockAdapter) *mocks.MockAdapter { return proto },
		},
		{
			name:     "Kafka JSON to JSON Schema",
			target:   projectdomain.Target{ID: "kafka-consumer:audit", Family: "kafka", Format: "json"},
			selected: func(_, _, jsonSchema *mocks.MockAdapter) *mocks.MockAdapter { return jsonSchema },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			openAPI := mocks.NewMockAdapter(ctrl)
			proto := mocks.NewMockAdapter(ctrl)
			jsonSchema := mocks.NewMockAdapter(ctrl)
			project := projectdomain.Project{Root: "/project"}
			expected := generatedomain.Output{Directory: artifact.Tree{Files: []artifact.File{{Path: "generated.go"}}}}
			test.selected(openAPI, proto, jsonSchema).EXPECT().Generate(gomock.Any(), project, test.target).Return(expected, nil)
			client := generatorclient.New(generatorclient.Adapters{
				OpenAPI: openAPI, Proto: proto, JSONSchema: jsonSchema,
			})

			actual, err := client.Generate(context.Background(), project, test.target)

			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}

func TestClientRejectsUnsupportedTarget(t *testing.T) {
	t.Parallel()

	client := generatorclient.New(generatorclient.Adapters{})

	_, err := client.Generate(context.Background(), projectdomain.Project{}, projectdomain.Target{
		ID: "kafka-consumer:audit", Family: "kafka", Format: "raw",
	})

	require.EqualError(t, err, `unsupported generation target "kafka-consumer:audit"`)
}

func TestClientPreservesAdapterFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	openAPI := mocks.NewMockAdapter(ctrl)
	cause := errors.New("tool failed")
	project := projectdomain.Project{Root: "/project"}
	target := projectdomain.Target{ID: "http-server", Family: "http", Format: "openapi"}
	openAPI.EXPECT().Generate(gomock.Any(), project, target).Return(generatedomain.Output{}, cause)
	client := generatorclient.New(generatorclient.Adapters{OpenAPI: openAPI})

	_, err := client.Generate(context.Background(), project, target)

	require.ErrorContains(t, err, "openAPI.Generate")
	require.ErrorIs(t, err, cause)
}
