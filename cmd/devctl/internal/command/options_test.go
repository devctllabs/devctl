package command

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func TestCommonCmdOptsBuildsCommandLoggers(t *testing.T) {
	t.Parallel()

	var opts CommonCmdOpts
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	leaf := &cli.Command{
		Name:  "validate",
		Flags: opts.CommonFlags(),
		Action: func(_ context.Context, command *cli.Command) error {
			opts.NewStdoutLogger(command).Info("completed", zap.String("stream", "stdout"))
			opts.NewStderrLogger(command).Debug("diagnostic", zap.String("stream", "stderr"))
			return nil
		},
	}
	root := &cli.Command{Name: "devctl", Commands: []*cli.Command{leaf}, Writer: &stdout, ErrWriter: &stderr}

	require.NoError(t, root.Run(context.Background(), []string{"devctl", "validate", "--json", "--verbose", "--file", "custom.yaml"}))
	require.Equal(t, "custom.yaml", opts.ManifestPath)
	require.True(t, opts.JSON)
	require.True(t, opts.Verbose)
	requireJSONLog(t, stdout.Bytes(), logExpectation{level: "info", command: "validate", message: "completed"})
	requireJSONLog(t, stderr.Bytes(), logExpectation{level: "debug", command: "validate", message: "diagnostic"})
}

type logExpectation struct {
	level   string
	command string
	message string
}

func requireJSONLog(t *testing.T, data []byte, expected logExpectation) {
	t.Helper()
	var event map[string]any
	require.NoError(t, json.Unmarshal(data, &event))
	require.Equal(t, expected.level, event["level"])
	require.Equal(t, expected.command, event["command"])
	require.Equal(t, expected.message, event["msg"])
}
