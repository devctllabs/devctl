package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devctllabs/devctl/cmd/devctl/internal/inspect/mocks"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestInspectActionEmitsEffectiveProjectEvent(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	inspector := mocks.NewMockprojectInspector(ctrl)
	inspector.EXPECT().Inspect(gomock.Any(), projectdomain.InspectQuery{ManifestPath: "custom.yaml"}).Return(projectdomain.InspectResult{
		Project: projectdomain.Inspection{
			Root: "/work", ManifestPath: "/work/devctl.yaml", Name: "sample", Language: "go",
			Module: "example.test/sample", EnvPrefix: "SAMPLE_", Targets: []projectdomain.InspectionTarget{{
				ID: "api", Family: "http", Format: "openapi", Input: "api/external/http/client/api",
				ResolvedInput: "api/external/http/client/api/openapi.yaml", Config: "tools/oapi/api.yaml",
			}},
			Paths: projectdomain.Paths{ExternalContracts: "api/external", ConfigOut: "gen/config", ServerOut: "gen/serverhttp", ClientOut: "gen/clienthttp"},
		},
	}, nil)
	command := newInspectCmd(inspectCmdOpts{}, func(context.Context, *zap.Logger) (inspectRuntime, error) {
		return inspectRuntime{inspector: inspector, shutdown: func(context.Context) error { return nil }}, nil
	})
	var stdout bytes.Buffer
	command.Writer = &stdout

	err := command.Run(context.Background(), []string{"inspect", "--file", "custom.yaml", "--json"})

	require.NoError(t, err)
	require.Contains(t, stdout.String(), `"msg":"project inspection completed"`)
	require.Contains(t, stdout.String(), `"command":"inspect"`)
	require.Contains(t, stdout.String(), `"data":{"project":{"root":"/work"`)
	require.Contains(t, stdout.String(), `"resolved_input":"api/external/http/client/api/openapi.yaml"`)
	require.Contains(t, stdout.String(), `"config":"tools/oapi/api.yaml"`)
	require.NotContains(t, stdout.String(), "next_steps")
	require.NotContains(t, stdout.String(), `"status"`)
}

func TestInspectActionReportsServiceErrorOnce(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	inspector := mocks.NewMockprojectInspector(ctrl)
	cause := errors.New("inspect failed")
	shutdownErr := errors.New("shutdown failed")
	inspector.EXPECT().Inspect(gomock.Any(), projectdomain.InspectQuery{}).Return(projectdomain.InspectResult{}, cause)
	command := newInspectCmd(inspectCmdOpts{}, func(context.Context, *zap.Logger) (inspectRuntime, error) {
		return inspectRuntime{inspector: inspector, shutdown: func(context.Context) error { return shutdownErr }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"inspect", "--json"})

	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, shutdownErr)
	require.Empty(t, err.Error())
	require.Equal(t, 1, bytes.Count(stderr.Bytes(), []byte(`"level":"error"`)))
	require.NotContains(t, stderr.String(), `"data"`)
}

func TestInspectActionReportsCompletedInspectionAfterShutdownFailure(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	inspector := mocks.NewMockprojectInspector(ctrl)
	shutdownErr := errors.New("shutdown failed")
	inspector.EXPECT().Inspect(gomock.Any(), projectdomain.InspectQuery{}).Return(projectdomain.InspectResult{
		Project: projectdomain.Inspection{Name: "sample"},
	}, nil)
	command := newInspectCmd(inspectCmdOpts{}, func(context.Context, *zap.Logger) (inspectRuntime, error) {
		return inspectRuntime{inspector: inspector, shutdown: func(context.Context) error { return shutdownErr }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"inspect", "--json"})

	require.ErrorIs(t, err, shutdownErr)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.NotContains(t, event, "data")
	details, ok := event["details"].(map[string]any)
	require.True(t, ok)
	partial, ok := details["partial_result"].(map[string]any)
	require.True(t, ok)
	project, ok := partial["project"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "sample", project["name"])
}
