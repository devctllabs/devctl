package contractsnapshot_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/service/contractsnapshot"
	"github.com/devctllabs/devctl/internal/service/contractsnapshot/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLoaderLoadsCommittedProtoSnapshot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	reader := mocks.NewMockReader(ctrl)
	loader := contractsnapshot.New(reader)
	const root = "/project"
	const treeRoot = "api/external/grpc/client/billing"
	metadata := []byte(`{"kind":"grpc","format":"proto","module_root":"api/proto/grpc","buf_config":"buf.yaml"}`)

	reader.EXPECT().ReadFile(gomock.Any(), root, treeRoot+"/.devctl-contract.json").Return(
		contract.File{Path: treeRoot + "/.devctl-contract.json", Content: metadata, Mode: 0o644}, nil,
	)
	reader.EXPECT().ReadFile(gomock.Any(), root, treeRoot+"/buf.yaml").Return(
		contract.File{Path: treeRoot + "/buf.yaml", Content: []byte("version: v2\n"), Mode: 0o644}, nil,
	)
	reader.EXPECT().ReadTree(gomock.Any(), root, treeRoot+"/api/proto/grpc").Return([]contract.File{{
		Path: treeRoot + "/api/proto/grpc/billing/v1/service.proto",
	}}, nil)
	reader.EXPECT().ReadTree(gomock.Any(), root, treeRoot).Return([]contract.File{
		{Path: treeRoot + "/buf.yaml", Content: []byte("version: v2\n"), Mode: 0o644},
		{Path: treeRoot + "/.devctl-contract.json", Content: metadata, Mode: 0o644},
		{Path: treeRoot + "/api/proto/grpc/billing/v1/service.proto", Content: []byte("syntax = \"proto3\";\n"), Mode: 0o600},
	}, nil)

	snapshot, err := loader.Load(
		context.Background(), root, treeRoot,
		contract.MetadataExpectation{Kind: "grpc", Format: "proto"},
	)

	require.NoError(t, err)
	require.Equal(t, "api/proto/grpc", snapshot.ModuleRoot)
	require.Empty(t, snapshot.Entrypoint)
	require.Equal(t, []contract.File{
		{Path: "api/proto/grpc/billing/v1/service.proto", Content: []byte("syntax = \"proto3\";\n"), Mode: 0o600},
		{Path: "buf.yaml", Content: []byte("version: v2\n"), Mode: 0o644},
	}, snapshot.Files)
	require.Equal(t, &contract.Metadata{
		Kind: "grpc", Format: "proto", ModuleRoot: "api/proto/grpc", BufConfig: "buf.yaml",
	}, snapshot.Metadata)
}

func TestLoaderAcceptsRawKafkaMetadataWithoutFiles(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	reader := mocks.NewMockReader(ctrl)
	loader := contractsnapshot.New(reader)
	const root = "/project"
	const treeRoot = "api/external/kafka/consumer/events"
	metadata := []byte(`{"kind":"kafka","topic":"sample.events.created.v1","format":"raw"}`)
	reader.EXPECT().ReadFile(gomock.Any(), root, treeRoot+"/.devctl-contract.json").Return(
		contract.File{Content: metadata}, nil,
	)
	reader.EXPECT().ReadTree(gomock.Any(), root, treeRoot).Return([]contract.File{{
		Path: treeRoot + "/.devctl-contract.json", Content: metadata, Mode: 0o644,
	}}, nil)

	snapshot, err := loader.Load(context.Background(), root, treeRoot, contract.MetadataExpectation{
		Kind: "kafka", Topic: "sample.events.created.v1", Format: "raw",
	})

	require.NoError(t, err)
	require.Empty(t, snapshot.Files)
	require.Empty(t, snapshot.Entrypoint)
	require.Empty(t, snapshot.ModuleRoot)
}

func TestLoaderReportsPreciseMetadataErrors(t *testing.T) {
	t.Parallel()

	errNotRegular := errors.New("not a regular file")
	tests := []struct {
		name     string
		load     func(*mocks.MockReader)
		expected contract.MetadataExpectation
		field    string
		reason   contract.MetadataInvalidReason
		cause    error
	}{
		{
			name: "missing sidecar",
			load: func(reader *mocks.MockReader) {
				reader.EXPECT().ReadFile(gomock.Any(), "/project", "contracts/.devctl-contract.json").Return(
					contract.File{}, fs.ErrNotExist,
				)
			},
			expected: contract.MetadataExpectation{Kind: "kafka", Format: "json"},
			field:    ".devctl-contract.json", reason: contract.MetadataRequired, cause: fs.ErrNotExist,
		},
		{
			name: "invalid metadata type",
			load: func(reader *mocks.MockReader) {
				reader.EXPECT().ReadFile(gomock.Any(), "/project", "contracts/.devctl-contract.json").Return(
					contract.File{Content: []byte(`{"kind":"kafka","topic":42,"format":"json","entrypoint":"schema.json"}`)}, nil,
				)
			},
			expected: contract.MetadataExpectation{Kind: "kafka", Format: "json"},
			field:    "topic", reason: contract.MetadataInvalidType,
		},
		{
			name: "missing entrypoint",
			load: func(reader *mocks.MockReader) {
				reader.EXPECT().ReadFile(gomock.Any(), "/project", "contracts/.devctl-contract.json").Return(
					contract.File{Content: []byte(`{"kind":"kafka","topic":"sample.events.created.v1","format":"json","entrypoint":"schema.json"}`)}, nil,
				)
				reader.EXPECT().ReadFile(gomock.Any(), "/project", "contracts/schema.json").Return(
					contract.File{}, fs.ErrNotExist,
				)
			},
			expected: contract.MetadataExpectation{Kind: "kafka", Topic: "sample.events.created.v1", Format: "json"},
			field:    "entrypoint", reason: contract.MetadataNotFound, cause: fs.ErrNotExist,
		},
		{
			name: "non regular buf config",
			load: func(reader *mocks.MockReader) {
				reader.EXPECT().ReadFile(gomock.Any(), "/project", "contracts/.devctl-contract.json").Return(
					contract.File{Content: []byte(`{"kind":"grpc","format":"proto","module_root":"proto","buf_config":"buf.yaml"}`)}, nil,
				)
				reader.EXPECT().ReadFile(gomock.Any(), "/project", "contracts/buf.yaml").Return(
					contract.File{}, errNotRegular,
				)
			},
			expected: contract.MetadataExpectation{Kind: "grpc", Format: "proto"},
			field:    "buf_config", reason: contract.MetadataNotRegular, cause: errNotRegular,
		},
		{
			name: "missing module root",
			load: func(reader *mocks.MockReader) {
				reader.EXPECT().ReadFile(gomock.Any(), "/project", "contracts/.devctl-contract.json").Return(
					contract.File{Content: []byte(`{"kind":"grpc","format":"proto","module_root":"proto","buf_config":"buf.yaml"}`)}, nil,
				)
				reader.EXPECT().ReadFile(gomock.Any(), "/project", "contracts/buf.yaml").Return(contract.File{}, nil)
				reader.EXPECT().ReadTree(gomock.Any(), "/project", "contracts/proto").Return(nil, fs.ErrNotExist)
			},
			expected: contract.MetadataExpectation{Kind: "grpc", Format: "proto"},
			field:    "module_root", reason: contract.MetadataNotFound, cause: fs.ErrNotExist,
		},
		{
			name: "cancelled tree read",
			load: func(reader *mocks.MockReader) {
				reader.EXPECT().ReadFile(gomock.Any(), "/project", "contracts/.devctl-contract.json").Return(
					contract.File{Content: []byte(`{"kind":"kafka","topic":"sample.events.created.v1","format":"raw"}`)}, nil,
				)
				reader.EXPECT().ReadTree(gomock.Any(), "/project", "contracts").Return(nil, context.Canceled)
			},
			expected: contract.MetadataExpectation{Kind: "kafka", Topic: "sample.events.created.v1", Format: "raw"},
			field:    "files", reason: contract.MetadataNotRegular, cause: context.Canceled,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			reader := mocks.NewMockReader(ctrl)
			test.load(reader)

			_, err := contractsnapshot.New(reader).Load(context.Background(), "/project", "contracts", test.expected)

			var metadataErr *contract.SnapshotMetadataError
			require.ErrorAs(t, err, &metadataErr)
			require.Equal(t, test.field, metadataErr.Field)
			require.Equal(t, test.reason, metadataErr.Reason)
			require.Equal(t, "devctl sync", metadataErr.Hint)
			if test.cause != nil {
				require.ErrorIs(t, err, test.cause)
			}
		})
	}
}
