package command

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/contract"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestErrorReporterWritesSafeEventAndReturnsSilentCause(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	command := &cli.Command{Name: "validate", ErrWriter: &stderr}
	opts := CommonCmdOpts{JSON: true}
	logger := opts.NewStderrLogger(command)
	reporter := NewErrorReporter(logger, false)
	cause := &projectdomain.OperationError{
		Operation: projectdomain.OperationLoadManifest,
		Path:      "/private/project/devctl.yaml",
		Kind:      projectdomain.FailureNotFound,
		Cause:     fs.ErrNotExist,
	}

	err := reporter.ReportError(cause, WithPartialResult(map[string]int{"completed": 2}))

	var exitCoder cli.ExitCoder
	require.ErrorAs(t, err, &exitCoder)
	require.Equal(t, 1, exitCoder.ExitCode())
	require.Empty(t, err.Error())
	require.ErrorIs(t, err, fs.ErrNotExist)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.Equal(t, "requested resource was not found", event["msg"])
	require.Equal(t, "not_found", event["code"])
	require.EqualValues(t, 1, event["exit_code"])
	require.NotContains(t, event, "data")
	require.Equal(t, map[string]any{
		"partial_result": map[string]any{"completed": float64(2)},
	}, event["details"])
	require.NotContains(t, event, "error")
	require.NotContains(t, stderr.String(), cause.Path)
}

func TestErrorReporterExposesSafeSnapshotRefreshDetails(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	command := &cli.Command{Name: "gen", ErrWriter: &stderr}
	opts := CommonCmdOpts{JSON: true}
	logger := opts.NewStderrLogger(command)
	reporter := NewErrorReporter(logger, false)
	cause := &contract.SnapshotMetadataError{
		Field: "entrypoint", Reason: contract.MetadataRequired, Hint: "devctl sync",
	}

	err := reporter.ReportError(cause, WithPartialResult(map[string]int{"completed": 1}))

	require.Error(t, err)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.Equal(t, "invalid_input", event["code"])
	require.Equal(t, map[string]any{
		"type": "snapshot_metadata_invalid", "field": "entrypoint",
		"reason": "required", "hint": "devctl sync",
		"partial_result": map[string]any{"completed": float64(1)},
	}, event["details"])
	require.NotContains(t, event, "data")
}

func TestErrorReporterMergesManifestIssuesWithPartialResult(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	command := &cli.Command{Name: "sync", ErrWriter: &stderr}
	opts := CommonCmdOpts{JSON: true}
	reporter := NewErrorReporter(opts.NewStderrLogger(command), false)
	cause := &projectdomain.InvalidManifestError{
		Path:   "devctl.yaml",
		Issues: []projectdomain.Issue{{Code: projectdomain.IssueCode("source_not_found"), Field: "sources.catalog"}},
	}

	err := reporter.ReportError(cause, WithPartialResult(map[string]int{"completed": 1}))

	require.Error(t, err)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.NotContains(t, event, "data")
	details, ok := event["details"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "devctl.yaml", details["path"])
	require.Equal(t, map[string]any{"completed": float64(1)}, details["partial_result"])
	require.Equal(t, []any{map[string]any{"code": "source_not_found", "field": "sources.catalog"}}, details["issues"])
}
