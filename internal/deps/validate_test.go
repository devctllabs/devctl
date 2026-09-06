package deps

import (
	"context"
	"testing"

	bufgenclient "github.com/devctllabs/devctl/internal/client/bufgen"
	generatorclient "github.com/devctllabs/devctl/internal/client/generator"
	"github.com/devctllabs/devctl/internal/client/toolrun"
	"github.com/devctllabs/devctl/internal/service/contractsnapshot"
	generateservice "github.com/devctllabs/devctl/internal/service/generate"
	lintservice "github.com/devctllabs/devctl/internal/service/lint"
	projectservice "github.com/devctllabs/devctl/internal/service/project"
	"github.com/devctllabs/devctl/internal/service/projectreadiness"
	"github.com/devctllabs/devctl/internal/service/targetinput"
	"github.com/devctllabs/go-libs/di"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewRegistersOneLazyGraphWithTypedRoots(t *testing.T) {
	t.Parallel()

	container, err := New(zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Shutdown(context.Background())) })

	projectService, err := container.ProjectService()
	require.NoError(t, err)
	require.NotNil(t, projectService)
	scaffoldService, err := container.ScaffoldService()
	require.NoError(t, err)
	require.NotNil(t, scaffoldService)
	syncService, err := container.SyncService()
	require.NoError(t, err)
	require.NotNil(t, syncService)
	lintService, err := container.LintService()
	require.NoError(t, err)
	require.NotNil(t, lintService)
	genService, err := container.GenService()
	require.NoError(t, err)
	require.NotNil(t, genService)
}

func TestNewRegistersTheCommandDiagnosticLogger(t *testing.T) {
	t.Parallel()

	logger := zap.NewExample()
	container, err := New(logger)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Shutdown(context.Background())) })
	resolved, err := di.Resolve[*zap.Logger](container.di)
	require.NoError(t, err)
	require.Same(t, logger, resolved)
}

func TestNewRegistersCompositeGeneratorAndSharesBufWithLint(t *testing.T) {
	t.Parallel()

	container, err := New(zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Shutdown(context.Background())) })

	generator, err := di.Resolve[generateservice.GeneratorClient](container.di)
	require.NoError(t, err)
	require.IsType(t, &generatorclient.Client{}, generator)
	buf, err := di.Resolve[*bufgenclient.Client](container.di)
	require.NoError(t, err)
	linter, err := di.Resolve[lintservice.ProtoLinter](container.di)
	require.NoError(t, err)
	require.Same(t, buf, linter)
}

func TestNewRegistersOneSharedToolRunner(t *testing.T) {
	t.Parallel()

	container, err := New(zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Shutdown(context.Background())) })

	osRunner, err := di.Resolve[*toolrun.OSRunner](container.di)
	require.NoError(t, err)
	runner, err := di.Resolve[toolrun.Runner](container.di)
	require.NoError(t, err)
	require.Same(t, osRunner, runner)
}

func TestNewRegistersSharedTargetInputServices(t *testing.T) {
	t.Parallel()

	container, err := New(zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Shutdown(context.Background())) })

	snapshots, err := di.Resolve[*contractsnapshot.Loader](container.di)
	require.NoError(t, err)
	require.NotNil(t, snapshots)
	inputs, err := di.Resolve[*targetinput.Resolver](container.di)
	require.NoError(t, err)
	require.NotNil(t, inputs)
}

func TestNewRegistersSplitProjectLocationAndReadinessServices(t *testing.T) {
	t.Parallel()

	container, err := New(zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Shutdown(context.Background())) })

	locator, err := di.Resolve[projectservice.ManifestLocator](container.di)
	require.NoError(t, err)
	readinessWorkspace, err := di.Resolve[projectreadiness.Workspace](container.di)
	require.NoError(t, err)
	require.Same(t, locator, readinessWorkspace)
	checker, err := di.Resolve[*projectreadiness.Checker](container.di)
	require.NoError(t, err)
	require.NotNil(t, checker)
}
