package enable

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/devctllabs/devctl/cmd/devctl/internal/enable/mocks"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestEnableWritesManifestChangeJSON(t *testing.T) {
	t.Parallel()
	manifestPath := writeManifest(t)
	command := newLoggingCmd(capabilityCmdOpts{}, buildCapability)
	var stdout bytes.Buffer
	command.Writer = &stdout

	err := command.Run(context.Background(), []string{"logging", "--file", manifestPath, "--json"})

	require.NoError(t, err)
	require.Contains(t, stdout.String(), `"msg":"capability enablement completed"`)
	require.Contains(t, stdout.String(), `"command":"logging"`)
	require.Contains(t, stdout.String(), `"data":{"manifest":"`+manifestPath+`","change":"updated"}`)
}

func TestEnableReportsOperationAndShutdownFailuresWithManifestData(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	enabler := mocks.NewMockcapabilityEnabler(ctrl)
	operationErr := errors.New("enable failed")
	shutdownErr := errors.New("shutdown failed")
	enabler.EXPECT().Enable(gomock.Any(), projectdomain.EnableCommand{Capability: "logging"}).Return(
		projectdomain.ManifestResult{Manifest: "/project/devctl.yaml", Change: projectdomain.ChangeUpdated}, operationErr,
	)
	command := newLoggingCmd(capabilityCmdOpts{}, func(context.Context, *zap.Logger) (capabilityRuntime, error) {
		return capabilityRuntime{enabler: enabler, shutdown: func(context.Context) error { return shutdownErr }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"logging", "--json"})

	require.ErrorIs(t, err, operationErr)
	require.ErrorIs(t, err, shutdownErr)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.NotContains(t, event, "data")
	details, ok := event["details"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"manifest": "/project/devctl.yaml", "change": "updated"}, details["partial_result"])
}

func writeManifest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))
	return path
}
