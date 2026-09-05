package targetinput_test

import (
	"context"
	"errors"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/targetinput"
	"github.com/devctllabs/devctl/internal/service/targetinput/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestResolverResolvesHTTPEntrypointInsideProject(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	entrypoints := mocks.NewMockEntrypointResolver(ctrl)
	snapshots := mocks.NewMockSnapshotLoader(ctrl)
	resolver := targetinput.New(entrypoints, snapshots)
	selected := project.Project{Root: "/project"}
	target := project.Target{
		ID: "http-server", Family: "http",
		Location: contract.Location{
			RelativePath: "api/openapi/swagger.yaml",
			Entrypoint:   "api/openapi/swagger.yaml",
			Local:        true,
		},
	}
	entrypoints.EXPECT().ResolveContract(gomock.Any(), contract.Location{
		Root:         selected.Root,
		RelativePath: target.Location.RelativePath,
		Entrypoint:   target.Location.Entrypoint,
		Local:        true,
	}).Return("/project/api/openapi/swagger.yaml", nil)

	resolved, err := resolver.Resolve(context.Background(), selected, target)

	require.NoError(t, err)
	require.Equal(t, "/project/api/openapi/swagger.yaml", resolved.Input)
	require.Equal(t, target.Location, resolved.Location)
}

func TestResolverResolvesExternalHTTPEntrypointInsideMaterializedTree(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	entrypoints := mocks.NewMockEntrypointResolver(ctrl)
	snapshots := mocks.NewMockSnapshotLoader(ctrl)
	target := project.Target{
		ID: "http-client:billing", Family: "http",
		Source: project.Source{Type: project.SourceURL},
		Location: contract.Location{
			RelativePath: "api/external/http/client/billing",
			Entrypoint:   "openapi.yaml",
		},
	}
	entrypoints.EXPECT().ResolveContract(gomock.Any(), contract.Location{
		Root:         "/project",
		RelativePath: target.Location.RelativePath,
		Entrypoint:   target.Location.Entrypoint,
	}).Return("/project/api/external/http/client/billing/openapi.yaml", nil)

	resolved, err := targetinput.New(entrypoints, snapshots).Resolve(
		context.Background(), project.Project{Root: "/project"}, target,
	)

	require.NoError(t, err)
	require.Equal(t, "/project/api/external/http/client/billing/openapi.yaml", resolved.Input)
	require.Equal(t, target.Location, resolved.Location)
}

func TestResolverResolvesCommittedSnapshotTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		target   project.Target
		snapshot contract.Snapshot
		input    string
		paths    []string
	}{
		{
			name: "grpc",
			target: project.Target{
				ID: "grpc-client:billing", Family: "grpc", Format: "proto",
				Source:   project.Source{Type: project.SourceDevctl},
				Location: contract.Location{RelativePath: "api/external/grpc/client/billing"},
			},
			snapshot: contract.Snapshot{ModuleRoot: "api/proto/grpc"},
			input:    "api/external/grpc/client/billing/api/proto/grpc",
		},
		{
			name: "kafka proto",
			target: project.Target{
				ID: "kafka-consumer:events", Family: "kafka", Format: "proto",
				Source:    project.Source{Type: project.SourceDevctl},
				Reference: contract.Reference{Topic: "sample.events.created.v1"},
				Location:  contract.Location{RelativePath: "api/external/kafka/consumer/events"},
			},
			snapshot: contract.Snapshot{ModuleRoot: "proto", Entrypoint: "proto/events.proto"},
			input:    "api/external/kafka/consumer/events/proto",
			paths:    []string{"events.proto"},
		},
		{
			name: "kafka json",
			target: project.Target{
				ID: "kafka-consumer:events", Family: "kafka", Format: "json",
				Source:    project.Source{Type: project.SourceDevctl},
				Reference: contract.Reference{Topic: "sample.events.created.v1"},
				Location:  contract.Location{RelativePath: "api/external/kafka/consumer/events"},
			},
			snapshot: contract.Snapshot{Entrypoint: "schemas/events.json"},
			input:    "api/external/kafka/consumer/events/schemas/events.json",
		},
		{
			name: "kafka raw",
			target: project.Target{
				ID: "kafka-consumer:events", Family: "kafka", Format: "raw",
				Source:    project.Source{Type: project.SourceDevctl},
				Reference: contract.Reference{Topic: "sample.events.created.v1"},
				Location:  contract.Location{RelativePath: "api/external/kafka/consumer/events"},
			},
			snapshot: contract.Snapshot{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			entrypoints := mocks.NewMockEntrypointResolver(ctrl)
			snapshots := mocks.NewMockSnapshotLoader(ctrl)
			selected := project.Project{Root: "/project"}
			snapshots.EXPECT().Load(
				gomock.Any(), selected.Root, test.target.Location.RelativePath, test.target.SnapshotExpectation(),
			).Return(test.snapshot, nil)

			resolved, err := targetinput.New(entrypoints, snapshots).Resolve(
				context.Background(), selected, test.target,
			)

			require.NoError(t, err)
			require.Equal(t, test.input, resolved.Input)
			require.Equal(t, test.paths, resolved.Paths)
			if test.snapshot.Entrypoint != "" {
				require.Equal(t, test.snapshot.Entrypoint, resolved.Location.Entrypoint)
			}
		})
	}
}

func TestResolverPassesThroughTargetsWithoutResolvableInput(t *testing.T) {
	t.Parallel()

	tests := []project.Target{
		{ID: "config", Family: "config", Format: "go"},
		{ID: "grpc-server", Family: "grpc", Format: "proto", Source: project.Source{Type: project.SourceLocal}},
		{ID: "kafka-producer:events", Family: "kafka", Format: "json", Source: project.Source{Type: project.SourceLocal}},
	}
	for _, target := range tests {
		target := target
		t.Run(target.ID, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			resolved, err := targetinput.New(
				mocks.NewMockEntrypointResolver(ctrl), mocks.NewMockSnapshotLoader(ctrl),
			).Resolve(context.Background(), project.Project{Root: "/project"}, target)

			require.NoError(t, err)
			require.Equal(t, target, resolved)
		})
	}
}

func TestResolverPreservesDependencyErrors(t *testing.T) {
	t.Parallel()

	t.Run("entrypoint", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		entrypoints := mocks.NewMockEntrypointResolver(ctrl)
		snapshots := mocks.NewMockSnapshotLoader(ctrl)
		cause := errors.New("entrypoint unavailable")
		target := project.Target{Family: "http", Location: contract.Location{RelativePath: "api", Entrypoint: "openapi.yaml"}}
		entrypoints.EXPECT().ResolveContract(gomock.Any(), contract.Location{
			Root: "/project", RelativePath: "api", Entrypoint: "openapi.yaml",
		}).Return("", cause)

		resolved, err := targetinput.New(entrypoints, snapshots).Resolve(
			context.Background(), project.Project{Root: "/project"}, target,
		)

		require.Equal(t, target, resolved)
		require.ErrorIs(t, err, cause)
	})

	t.Run("snapshot", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		entrypoints := mocks.NewMockEntrypointResolver(ctrl)
		snapshots := mocks.NewMockSnapshotLoader(ctrl)
		cause := errors.New("snapshot unavailable")
		target := project.Target{
			Family: "grpc", Format: "proto", Source: project.Source{Type: project.SourceDevctl},
			Location: contract.Location{RelativePath: "contracts"},
		}
		snapshots.EXPECT().Load(
			gomock.Any(), "/project", "contracts", target.SnapshotExpectation(),
		).Return(contract.Snapshot{}, cause)

		resolved, err := targetinput.New(entrypoints, snapshots).Resolve(
			context.Background(), project.Project{Root: "/project"}, target,
		)

		require.Equal(t, target, resolved)
		require.ErrorIs(t, err, cause)
	})
}
