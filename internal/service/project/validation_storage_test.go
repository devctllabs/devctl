package project_test

import (
	"context"
	"testing"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/project/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestServiceValidateRejectsS3BucketWithUnknownConnection(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Components: projectdomain.Components{S3: &projectdomain.S3{
				Connections: []projectdomain.S3Connection{{Name: "default", Credentials: "static"}},
				Buckets:     []projectdomain.S3Bucket{{Name: "media", Connection: "archive", Bucket: "media-local"}},
			}},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := newValidationService(ctrl, manifests, workspace, selected).Validate(
		context.Background(), projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"},
	)

	require.NoError(t, err)
	require.Equal(t, projectdomain.ValidationResult{Issues: []projectdomain.Issue{{
		Code: "s3_connection_not_found", Path: "/project/devctl.yaml",
		Field: "components.s3.buckets.media.connection",
	}}}, result)
}

func TestServiceValidateRejectsInvalidRedisAddresses(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Components: projectdomain.Components{Redis: &projectdomain.Redis{Connections: []projectdomain.RedisConnection{
				{Name: "cache", AddrEnv: "REDIS_CACHE_ADDR", AddrDefault: "localhost"},
				{Name: "sessions", AddrEnv: "REDIS_SESSIONS_ADDR", AddrDefault: "redis://user:secret@localhost:6379/0"},
			}}},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := newValidationService(ctrl, manifests, workspace, selected).Validate(
		context.Background(), projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"},
	)

	require.NoError(t, err)
	require.Equal(t, []projectdomain.Issue{
		{Code: "redis_address_invalid", Path: "/project/devctl.yaml", Field: "components.redis.connections.cache.addr_default"},
		{Code: "redis_address_invalid", Path: "/project/devctl.yaml", Field: "components.redis.connections.sessions.addr_default"},
	}, result.Issues)
}
