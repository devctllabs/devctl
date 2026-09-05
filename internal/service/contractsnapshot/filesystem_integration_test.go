package contractsnapshot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/failure"
	workspacerepo "github.com/devctllabs/devctl/internal/repository/workspace"
	"github.com/devctllabs/devctl/internal/service/contractsnapshot"
	"github.com/stretchr/testify/require"
)

func TestLoaderUsesFilesystemReaderForCommittedProtoSnapshot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	snapshotRoot := "api/external/grpc/client/billing"
	writeSnapshotFile(t, root, snapshotRoot+"/.devctl-contract.json", `{
  "kind": "grpc",
  "format": "proto",
  "module_root": "api/proto/grpc",
  "buf_config": "buf.yaml"
}`)
	writeSnapshotFile(t, root, snapshotRoot+"/api/proto/grpc/billing/v1/service.proto", "syntax = \"proto3\";\n")
	writeSnapshotFile(t, root, snapshotRoot+"/buf.yaml", "version: v2\n")
	writeSnapshotFile(t, root, snapshotRoot+"/buf.lock", "deps: []\n")

	snapshot, err := contractsnapshot.New(workspacerepo.NewFilesystemRepo()).Load(
		context.Background(), root, snapshotRoot,
		contract.MetadataExpectation{Kind: "grpc", Format: "proto"},
	)

	require.NoError(t, err)
	require.Equal(t, "api/proto/grpc", snapshot.ModuleRoot)
	require.Empty(t, snapshot.Entrypoint)
	require.Equal(t, []string{
		"api/proto/grpc/billing/v1/service.proto", "buf.lock", "buf.yaml",
	}, snapshotFilePaths(snapshot))
	require.Equal(t, "buf.yaml", snapshot.Metadata.BufConfig)
}

func TestLoaderUsesFilesystemReaderForTypedInvalidMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		metadata    string
		prepare     func(*testing.T, string, string)
		field       string
		reason      contract.MetadataInvalidReason
		expectation contract.MetadataExpectation
	}{
		{
			name: "missing sidecar", field: ".devctl-contract.json", reason: contract.MetadataRequired,
			expectation: contract.MetadataExpectation{Kind: "kafka", Format: "json"},
		},
		{
			name: "wrong JSON type", metadata: `{"kind":"kafka","topic":42,"format":"json","entrypoint":"schema.json"}`,
			field: "topic", reason: contract.MetadataInvalidType,
			expectation: contract.MetadataExpectation{Kind: "kafka", Format: "json"},
		},
		{
			name: "missing JSON entrypoint", metadata: `{"kind":"kafka","topic":"sample.events.created.v1","format":"json"}`,
			field: "entrypoint", reason: contract.MetadataRequired,
			expectation: contract.MetadataExpectation{Kind: "kafka", Format: "json"},
		},
		{
			name: "absolute entrypoint", metadata: `{"kind":"kafka","topic":"sample.events.created.v1","format":"json","entrypoint":"/schema.json"}`,
			field: "entrypoint", reason: contract.MetadataInvalidPath,
			expectation: contract.MetadataExpectation{Kind: "kafka", Format: "json"},
		},
		{
			name: "traversing module root", metadata: `{"kind":"grpc","format":"proto","module_root":"../proto","buf_config":"buf.yaml"}`,
			field: "module_root", reason: contract.MetadataInvalidPath,
			expectation: contract.MetadataExpectation{Kind: "grpc", Format: "proto"},
		},
		{
			name: "missing referenced file", metadata: `{"kind":"kafka","topic":"sample.events.created.v1","format":"json","entrypoint":"schema.json"}`,
			field: "entrypoint", reason: contract.MetadataNotFound,
			expectation: contract.MetadataExpectation{Kind: "kafka", Format: "json"},
		},
		{
			name: "symlink entrypoint", metadata: `{"kind":"kafka","topic":"sample.events.created.v1","format":"json","entrypoint":"schema.json"}`,
			prepare: func(t *testing.T, root, snapshotRoot string) {
				t.Helper()
				writeSnapshotFile(t, root, snapshotRoot+"/real.json", `{"title":"Event"}`)
				link := filepath.Join(root, filepath.FromSlash(snapshotRoot), "schema.json")
				require.NoError(t, os.Symlink("real.json", link))
			},
			field: "entrypoint", reason: contract.MetadataNotRegular,
			expectation: contract.MetadataExpectation{Kind: "kafka", Format: "json"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			const snapshotRoot = "api/external/kafka/consumer/events"
			if test.metadata != "" {
				writeSnapshotFile(t, root, snapshotRoot+"/.devctl-contract.json", test.metadata)
			}
			if test.prepare != nil {
				test.prepare(t, root, snapshotRoot)
			}

			_, err := contractsnapshot.New(workspacerepo.NewFilesystemRepo()).Load(
				context.Background(), root, snapshotRoot, test.expectation,
			)

			require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
			var metadataErr *contract.SnapshotMetadataError
			require.ErrorAs(t, err, &metadataErr)
			require.Equal(t, test.field, metadataErr.Field)
			require.Equal(t, test.reason, metadataErr.Reason)
			require.Equal(t, "devctl sync", metadataErr.Hint)
		})
	}
}

func TestLoaderUsesFilesystemReaderForRawKafkaWithoutFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	const snapshotRoot = "api/external/kafka/consumer/events"
	writeSnapshotFile(t, root, snapshotRoot+"/.devctl-contract.json", `{
  "kind":"kafka",
  "topic":"sample.events.created.v1",
  "format":"raw"
}`)

	snapshot, err := contractsnapshot.New(workspacerepo.NewFilesystemRepo()).Load(
		context.Background(), root, snapshotRoot,
		contract.MetadataExpectation{Kind: "kafka", Topic: "sample.events.created.v1", Format: "raw"},
	)

	require.NoError(t, err)
	require.Empty(t, snapshot.Files)
	require.Empty(t, snapshot.Entrypoint)
	require.Empty(t, snapshot.ModuleRoot)
}

func writeSnapshotFile(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(content), 0o644))
}

func snapshotFilePaths(snapshot contract.Snapshot) []string {
	paths := make([]string, len(snapshot.Files))
	for index, file := range snapshot.Files {
		paths[index] = file.Path
	}
	return paths
}
