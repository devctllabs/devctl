package materialize_test

import (
	"context"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/failure"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	"github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/materialize"
	"github.com/stretchr/testify/require"
)

type strategy struct {
	typeName project.SourceType
	called   bool
}

func (s *strategy) SourceType() project.SourceType { return s.typeName }

func (s *strategy) Materialize(context.Context, materializedomain.Request) (contract.Snapshot, error) {
	s.called = true
	return contract.Snapshot{Entrypoint: "openapi.yaml", Files: []contract.File{{Path: "openapi.yaml", Content: []byte("openapi: 3.1.0")}}}, nil
}

func TestRouterAcceptsStrategySubsetAndRoutesBySourceType(t *testing.T) {
	t.Parallel()

	local := &strategy{typeName: project.SourceLocal}
	service, err := materialize.New(local)
	require.NoError(t, err)

	snapshot, err := service.Materialize(
		context.Background(),
		"",
		project.Source{Type: project.SourceLocal},
		contract.Reference{Entrypoint: "openapi.yaml"},
	)

	require.NoError(t, err)
	require.True(t, local.called)
	require.Equal(t, "openapi.yaml", snapshot.Entrypoint)
}

func TestRouterRejectsDuplicateSourceType(t *testing.T) {
	t.Parallel()

	_, err := materialize.New(&strategy{typeName: project.SourceURL}, &strategy{typeName: project.SourceURL})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
}

func TestRouterReportsMissingStrategyAtCallTime(t *testing.T) {
	t.Parallel()

	service, err := materialize.New()
	require.NoError(t, err)

	_, err = service.Materialize(context.Background(), "", project.Source{Type: project.SourceGit}, contract.Reference{})

	require.Equal(t, failure.Unsupported, failure.CategoryOf(err))
	var unsupported *materializedomain.UnsupportedSourceError
	require.ErrorAs(t, err, &unsupported)
	require.Equal(t, project.SourceGit, unsupported.SourceType)
}
