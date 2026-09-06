package materialize_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/failure"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	"github.com/devctllabs/devctl/internal/domain/project"
	workspacerepo "github.com/devctllabs/devctl/internal/repository/workspace"
	"github.com/devctllabs/devctl/internal/service/materialize"
	"github.com/devctllabs/devctl/internal/service/materialize/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLocalMaterializesReferenceClosure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	reader := mocks.NewMockFileReader(ctrl)
	reader.EXPECT().ReadFile(gomock.Any(), "/project/api/contracts", "openapi.yaml").Return(contract.File{
		Content: []byte("openapi: 3.1.0\ncomponents:\n  schemas:\n    Item:\n      $ref: './components.yaml#/Item'\n"), Mode: 0o644,
	}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/project/api/contracts", "components.yaml").Return(contract.File{
		Content: []byte("Item:\n  type: object\n"), Mode: 0o644,
	}, nil)

	snapshot, err := materialize.NewLocal(reader).Materialize(context.Background(), materializedomain.Request{
		Root:      "/project",
		Source:    project.Source{Type: project.SourceLocal, Path: "api/contracts"},
		Reference: contract.Reference{Entrypoint: "openapi.yaml"},
	})

	require.NoError(t, err)
	require.Equal(t, "openapi.yaml", snapshot.Entrypoint)
	require.Len(t, snapshot.Files, 2)
}

func TestLocalMaterializesProtoRootTreeAndBufMetadata(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	reader := mocks.NewMockFileReader(ctrl)
	reader.EXPECT().ReadTree(gomock.Any(), "/project/api/contracts", "proto").Return([]contract.File{
		{Path: "proto/acme/v1/common.proto", Content: []byte("syntax = \"proto3\";\n"), Mode: 0o644},
		{Path: "proto/acme/v1/service.proto", Content: []byte("syntax = \"proto3\";\n"), Mode: 0o644},
	}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/project/api/contracts", "buf.yaml").Return(contract.File{
		Path: "buf.yaml", Content: []byte("version: v2\n"), Mode: 0o644,
	}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/project/api/contracts", "buf.lock").Return(contract.File{}, fs.ErrNotExist)

	snapshot, err := materialize.NewLocal(reader).Materialize(context.Background(), materializedomain.Request{
		Root: "/project",
		Source: project.Source{Type: project.SourceLocal, Path: "api/contracts", Proto: project.SourceProto{
			BufConfig: "buf.yaml",
		}},
		Reference: contract.Reference{Entrypoint: "proto/acme/v1/service.proto", Format: "proto", ProtoRoot: "proto"},
	})

	require.NoError(t, err)
	require.Equal(t, "proto/acme/v1/service.proto", snapshot.Entrypoint)
	require.Equal(t, []string{"buf.yaml", "proto/acme/v1/common.proto", "proto/acme/v1/service.proto"}, snapshotPaths(snapshot))
}

func TestLocalIncludesBufConfigAndAdjacentLockOnlyOnceWhenTheyAreInsideProtoRoot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	reader := mocks.NewMockFileReader(ctrl)
	reader.EXPECT().ReadTree(gomock.Any(), "/project/api/contracts", "proto").Return([]contract.File{
		{Path: "proto/acme/v1/service.proto", Content: []byte("syntax = \"proto3\";\n"), Mode: 0o644},
		{Path: "proto/buf.lock", Content: []byte("deps: []\n"), Mode: 0o644},
		{Path: "proto/buf.yaml", Content: []byte("version: v2\n"), Mode: 0o644},
	}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/project/api/contracts", "proto/buf.yaml").Return(contract.File{
		Content: []byte("version: v2\n"), Mode: 0o644,
	}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/project/api/contracts", "proto/buf.lock").Return(contract.File{
		Content: []byte("deps: []\n"), Mode: 0o644,
	}, nil)

	snapshot, err := materialize.NewLocal(reader).Materialize(context.Background(), materializedomain.Request{
		Root: "/project",
		Source: project.Source{Type: project.SourceLocal, Path: "api/contracts", Proto: project.SourceProto{
			BufConfig: "proto/buf.yaml",
		}},
		Reference: contract.Reference{Entrypoint: "proto/acme/v1/service.proto", Format: "proto", ProtoRoot: "proto"},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"proto/acme/v1/service.proto", "proto/buf.lock", "proto/buf.yaml"}, snapshotPaths(snapshot))
}

func TestLocalReportsInvalidBufConfigFileAtTheSelectedPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   string
		prepare  func(*testing.T, string)
		category failure.Category
	}{
		{
			name: "missing", config: "buf/missing.yaml", category: failure.NotFound,
			prepare: func(*testing.T, string) {},
		},
		{
			name: "directory", config: "buf/config", category: failure.Unavailable,
			prepare: func(t *testing.T, root string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(root, "contracts/buf/config"), 0o755))
			},
		},
		{
			name: "symlink", config: "buf/link.yaml", category: failure.Unavailable,
			prepare: func(t *testing.T, root string) {
				t.Helper()
				target := filepath.Join(root, "contracts/buf/target.yaml")
				require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
				require.NoError(t, os.WriteFile(target, []byte("version: v2\n"), 0o644))
				require.NoError(t, os.Symlink("target.yaml", filepath.Join(root, "contracts/buf/link.yaml")))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			proto := filepath.Join(root, "contracts/proto/service.proto")
			require.NoError(t, os.MkdirAll(filepath.Dir(proto), 0o755))
			require.NoError(t, os.WriteFile(proto, []byte("syntax = \"proto3\";\n"), 0o644))
			test.prepare(t, root)

			_, err := materialize.NewLocal(workspacerepo.NewFilesystemRepo()).Materialize(
				context.Background(),
				materializedomain.Request{
					Root: root,
					Source: project.Source{Type: project.SourceLocal, Path: "contracts", Proto: project.SourceProto{
						BufConfig: test.config,
					}},
					Reference: contract.Reference{Entrypoint: "proto/service.proto", Format: "proto", ProtoRoot: "proto"},
				},
			)

			require.Equal(t, test.category, failure.CategoryOf(err))
			var operationErr *materializedomain.OperationError
			require.ErrorAs(t, err, &operationErr)
			require.Equal(t, materializedomain.OperationReadFile, operationErr.Operation)
			require.Equal(t, test.config, operationErr.Path)
		})
	}
}

func snapshotPaths(snapshot contract.Snapshot) []string {
	paths := make([]string, 0, len(snapshot.Files))
	for _, file := range snapshot.Files {
		paths = append(paths, file.Path)
	}
	return paths
}

func TestURLRejectsInsecureSourceBeforeClientCall(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)

	_, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source: project.Source{Type: project.SourceURL, URL: "http://example.test/openapi.yaml"},
	})

	require.Error(t, err)
}

func TestURLRejectsCredentialedSourceWithoutLeakingCredentialsOrQuery(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)

	_, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source: project.Source{
			Type: project.SourceURL,
			URL:  "https://user:password@example.test/openapi.yaml?token=secret",
		},
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
	var operationErr *materializedomain.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Equal(t, "https://example.test/openapi.yaml", operationErr.Path)
	require.NotContains(t, err.Error(), "password")
	require.NotContains(t, err.Error(), "token=")
}

func TestURLBuildsSnapshotFromDownloadedDocument(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: "https://example.test/openapi.yaml", OriginURL: "https://example.test/openapi.yaml",
	}).Return(materializedomain.HTTPDocument{
		URL: "https://example.test/openapi.yaml", Content: []byte("openapi: 3.1.0\n"),
	}, nil)

	snapshot, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: "https://example.test/openapi.yaml"},
		Reference: contract.Reference{Entrypoint: "spec/openapi.yaml"},
	})

	require.NoError(t, err)
	require.Equal(t, "spec/openapi.yaml", snapshot.Entrypoint)
	require.Equal(t, []byte("openapi: 3.1.0\n"), snapshot.Files[0].Content)
}

func TestURLMaterializesRelativeReferenceClosureInFetchAndVirtualPaths(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	const sourceURL = "https://example.test/contracts/openapi.yaml"
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: sourceURL, OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL: sourceURL,
		Content: []byte("openapi: 3.1.0\ncomponents:\n  schemas:\n    Item:\n" +
			"      $ref: './schemas/common.yaml#/Item'\n"),
	}, nil)
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: "https://example.test/contracts/schemas/common.yaml", OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL:     "https://example.test/contracts/schemas/common.yaml",
		Content: []byte("Item:\n  type: object\n"),
	}, nil)

	snapshot, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: sourceURL},
		Reference: contract.Reference{Entrypoint: "spec/openapi.yaml"},
	})

	require.NoError(t, err)
	require.Equal(t, "spec/openapi.yaml", snapshot.Entrypoint)
	require.Equal(t, []string{"spec/openapi.yaml", "spec/schemas/common.yaml"}, snapshotPaths(snapshot))
}

func TestURLMaterializesJSONSchemaReferenceClosure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	const sourceURL = "https://example.test/contracts/event.json"
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: sourceURL, OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL: sourceURL, Content: []byte(`{"$ref":"./schemas/payload.json#/$defs/Payload"}`),
	}, nil)
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: "https://example.test/contracts/schemas/payload.json", OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL:     "https://example.test/contracts/schemas/payload.json",
		Content: []byte(`{"$defs":{"Payload":{"type":"object"}}}`),
	}, nil)

	snapshot, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: sourceURL},
		Reference: contract.Reference{Entrypoint: "schemas/event.json", Format: "json"},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"schemas/event.json", "schemas/schemas/payload.json"}, snapshotPaths(snapshot))
}

func TestURLResolvesReferenceFromEffectiveURLAfterRedirect(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	const sourceURL = "https://example.test/contracts/start"
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: sourceURL, OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL:     "https://example.test/redirected/openapi.yaml",
		Content: []byte("$ref: './schema.yaml'\n"),
	}, nil)
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: "https://example.test/redirected/schema.yaml", OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL: "https://example.test/redirected/schema.yaml", Content: []byte("type: object\n"),
	}, nil)

	snapshot, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: sourceURL},
		Reference: contract.Reference{Entrypoint: "spec/openapi.yaml"},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"spec/openapi.yaml", "spec/schema.yaml"}, snapshotPaths(snapshot))
}

func TestURLRejectsReferenceEscapingVirtualSourceRoot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	const sourceURL = "https://example.test/contracts/openapi.yaml"
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: sourceURL, OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL:     sourceURL,
		Content: []byte("openapi: 3.1.0\ncomponents:\n  schemas:\n    Item:\n      $ref: '../shared.yaml#/Item'\n"),
	}, nil)

	_, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: sourceURL},
		Reference: contract.Reference{Entrypoint: "spec/openapi.yaml"},
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
}

func TestURLTerminatesCyclesAndIgnoresAbsoluteReferences(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	const sourceURL = "https://example.test/contracts/openapi.yaml"
	gomock.InOrder(
		client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
			URL: sourceURL, OriginURL: sourceURL,
		}).Return(materializedomain.HTTPDocument{
			URL: sourceURL,
			Content: []byte("openapi: 3.1.0\nrefs:\n" +
				"  - {$ref: './schemas/common.yaml#/Item'}\n" +
				"  - {$ref: '#/components/schemas/Local'}\n" +
				"  - {$ref: 'https://example.test/contracts/ignored.yaml'}\n" +
				"  - {$ref: '/contracts/ignored.yaml'}\n" +
				"  - {$ref: '//example.test/contracts/ignored.yaml'}\n"),
		}, nil),
		client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
			URL: "https://example.test/contracts/schemas/common.yaml", OriginURL: sourceURL,
		}).Return(materializedomain.HTTPDocument{
			URL:     "https://example.test/contracts/schemas/common.yaml",
			Content: []byte("Item:\n  $ref: '../openapi.yaml#/components/schemas/Local'\n"),
		}, nil),
	)

	snapshot, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: sourceURL},
		Reference: contract.Reference{Entrypoint: "spec/openapi.yaml"},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"spec/openapi.yaml", "spec/schemas/common.yaml"}, snapshotPaths(snapshot))
}

func TestURLRejectsQueryIdentitiesCollidingOnVirtualPathWithoutLeakingQuery(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	const sourceURL = "https://example.test/contracts/openapi.yaml"
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: sourceURL, OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL: sourceURL,
		Content: []byte("refs:\n" +
			"  - {$ref: './schema.yaml?token=alpha#/Item'}\n" +
			"  - {$ref: './schema.yaml?token=bravo#/Item'}\n"),
	}, nil)

	_, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: sourceURL},
		Reference: contract.Reference{Entrypoint: "spec/openapi.yaml"},
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
	var operationErr *materializedomain.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Equal(t, "spec/schema.yaml", operationErr.Path)
	require.NotContains(t, err.Error(), "token=")
}

func TestURLPreservesDownloadCategoryAndRedactsQueryFromFailureContext(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	const sourceURL = "https://example.test/contracts/openapi.yaml"
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: sourceURL, OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL: sourceURL, Content: []byte("$ref: './missing.yaml?token=secret'\n"),
	}, nil)
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: "https://example.test/contracts/missing.yaml?token=secret", OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{}, &materializedomain.OperationError{
		Operation: materializedomain.OperationDownload,
		Kind:      materializedomain.FailureNotFound,
		Cause:     errors.New("HTTP status 404"),
	})

	_, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: sourceURL},
		Reference: contract.Reference{Entrypoint: "spec/openapi.yaml"},
	})

	require.Equal(t, failure.NotFound, failure.CategoryOf(err))
	var operationErr *materializedomain.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Equal(t, "https://example.test/contracts/missing.yaml", operationErr.Path)
	require.NotContains(t, err.Error(), "token=")
}

func TestURLRejectsClosureExceedingDocumentLimit(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	const sourceURL = "https://example.test/contracts/openapi.yaml"
	var refs strings.Builder
	refs.WriteString("refs:\n")
	for index := 0; index < 64; index++ {
		_, _ = fmt.Fprintf(&refs, "  - {$ref: './schemas/%02d.yaml'}\n", index)
	}
	fetches := 0
	client.EXPECT().Fetch(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, request materializedomain.HTTPFetchRequest) (materializedomain.HTTPDocument, error) {
			fetches++
			if request.URL == sourceURL {
				return materializedomain.HTTPDocument{URL: sourceURL, Content: []byte(refs.String())}, nil
			}
			return materializedomain.HTTPDocument{URL: request.URL, Content: []byte("type: object\n")}, nil
		},
	)

	_, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: sourceURL},
		Reference: contract.Reference{Entrypoint: "spec/openapi.yaml"},
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
	require.LessOrEqual(t, fetches, 64)
}

func TestURLAllowsClosureAtDocumentLimit(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	const sourceURL = "https://example.test/contracts/openapi.yaml"
	var refs strings.Builder
	refs.WriteString("refs:\n")
	for index := 0; index < 63; index++ {
		_, _ = fmt.Fprintf(&refs, "  - {$ref: './schemas/%02d.yaml'}\n", index)
	}
	fetches := 0
	client.EXPECT().Fetch(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, request materializedomain.HTTPFetchRequest) (materializedomain.HTTPDocument, error) {
			fetches++
			if request.URL == sourceURL {
				return materializedomain.HTTPDocument{URL: sourceURL, Content: []byte(refs.String())}, nil
			}
			return materializedomain.HTTPDocument{URL: request.URL, Content: []byte("type: object\n")}, nil
		},
	)

	snapshot, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: sourceURL},
		Reference: contract.Reference{Entrypoint: "spec/openapi.yaml"},
	})

	require.NoError(t, err)
	require.Len(t, snapshot.Files, 64)
	require.Equal(t, 64, fetches)
}

func TestURLRejectsDocumentExceedingResponseLimit(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	const sourceURL = "https://example.test/contracts/openapi.yaml"
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: sourceURL, OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL: sourceURL, Content: make([]byte, (32<<20)+1),
	}, nil)

	_, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: sourceURL},
		Reference: contract.Reference{Entrypoint: "spec/openapi.yaml"},
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
}

func TestURLRejectsClosureExceedingAggregateSizeLimit(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	const sourceURL = "https://example.test/contracts/openapi.yaml"
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: sourceURL, OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL:     sourceURL,
		Content: []byte("refs:\n  - {$ref: './one.yaml'}\n  - {$ref: './two.yaml'}\n"),
	}, nil)
	largeDocument := make([]byte, 32<<20)
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: "https://example.test/contracts/one.yaml", OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL: "https://example.test/contracts/one.yaml", Content: largeDocument,
	}, nil)
	client.EXPECT().Fetch(gomock.Any(), materializedomain.HTTPFetchRequest{
		URL: "https://example.test/contracts/two.yaml", OriginURL: sourceURL,
	}).Return(materializedomain.HTTPDocument{
		URL: "https://example.test/contracts/two.yaml", Content: largeDocument,
	}, nil)

	_, err := materialize.NewURL(client).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceURL, URL: sourceURL},
		Reference: contract.Reference{Entrypoint: "spec/openapi.yaml"},
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
}

func TestGitMaterializesCheckedOutReferenceClosure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "main", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout/contracts", "openapi.yaml").Return(contract.File{Content: []byte("openapi: 3.1.0\n")}, nil)

	snapshot, err := materialize.NewGit(client, reader).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceGit, Path: "contracts", Repo: "repo", Ref: "main"},
		Reference: contract.Reference{Entrypoint: "openapi.yaml"},
	})

	require.NoError(t, err)
	require.Equal(t, "openapi.yaml", snapshot.Entrypoint)
}

func TestDevctlMaterializesSelectedExportWithoutUpstreamReadiness(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Exports: map[string]project.Export{
			"public-api": {Kind: "openapi", Path: "api/openapi.yaml"},
		},
		Components: project.Components{HTTP: &project.HTTP{Server: &project.HTTPServer{OpenAPI: "api/openapi.yaml"}}},
	}}}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", "api/openapi.yaml").Return(contract.File{Content: []byte("openapi: 3.1.0\n")}, nil)

	snapshot, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{Export: "public-api"},
	})

	require.NoError(t, err)
	require.Equal(t, "api/openapi.yaml", snapshot.Entrypoint)
}

func TestDevctlRejectsSelectedExportThatDoesNotMatchEffectiveSurface(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Exports: map[string]project.Export{
			"public-api": {Kind: "openapi", Path: "api/exported.yaml"},
		},
		Components: project.Components{HTTP: &project.HTTP{Server: &project.HTTPServer{OpenAPI: "api/canonical.yaml"}}},
	}}}, nil)

	_, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{Export: "public-api"},
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
}

func TestDevctlMaterializesNamedGRPCExportTree(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Exports: map[string]project.Export{
			"billing": {Kind: "grpc", Path: "api/proto/grpc"},
		},
		Components: project.Components{GRPC: &project.GRPC{Server: &project.GRPCServer{ProtoRoot: "api/proto/grpc"}}},
	}}}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", "api/proto/grpc/.devctl-contract.json").Return(
		contract.File{}, fs.ErrNotExist,
	)
	reader.EXPECT().ReadTree(gomock.Any(), "/checkout", "api/proto/grpc").Return([]contract.File{{
		Path: "api/proto/grpc/acme/billing/v1/service.proto", Content: []byte("syntax = \"proto3\";\n"), Mode: 0o644,
	}}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", "buf.yaml").Return(contract.File{
		Content: []byte("version: v2\n"), Mode: 0o644,
	}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", "buf.lock").Return(contract.File{
		Content: []byte("deps: []\n"), Mode: 0o644,
	}, nil)

	snapshot, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{Export: "billing", Format: "proto"},
	})

	require.NoError(t, err)
	require.Empty(t, snapshot.Entrypoint)
	require.Equal(t, []string{"api/proto/grpc/acme/billing/v1/service.proto", "buf.lock", "buf.yaml"}, snapshotPaths(snapshot))
	require.Equal(t, &contract.Metadata{
		Kind: "grpc", Format: "proto", ModuleRoot: "api/proto/grpc", BufConfig: "buf.yaml",
	}, snapshot.Metadata)
}

func TestDevctlMaterializesExplicitGRPCBufConfigAndAdjacentLock(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Exports: map[string]project.Export{"billing": {Kind: "grpc", Path: "api/proto/grpc"}},
		Components: project.Components{GRPC: &project.GRPC{Server: &project.GRPCServer{
			ProtoRoot: "api/proto/grpc", BufConfig: "tools/buf/upstream.yaml",
		}}},
	}}}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", "api/proto/grpc/.devctl-contract.json").Return(
		contract.File{}, fs.ErrNotExist,
	)
	reader.EXPECT().ReadTree(gomock.Any(), "/checkout", "api/proto/grpc").Return([]contract.File{{
		Path: "api/proto/grpc/service.proto", Content: []byte("syntax = \"proto3\";\n"), Mode: 0o644,
	}}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", "tools/buf/upstream.yaml").Return(contract.File{
		Content: []byte("version: v2\n"), Mode: 0o644,
	}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", "tools/buf/buf.lock").Return(contract.File{
		Content: []byte("deps: []\n"), Mode: 0o644,
	}, nil)

	snapshot, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{Export: "billing", Format: "proto"},
	})

	require.NoError(t, err)
	require.Equal(t, []string{
		"api/proto/grpc/service.proto", "tools/buf/buf.lock", "tools/buf/upstream.yaml",
	}, snapshotPaths(snapshot))
	require.Equal(t, "tools/buf/upstream.yaml", snapshot.Metadata.BufConfig)
}

func TestDevctlReexportsCommittedGRPCSnapshotWithoutInventingEntrypoint(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	const treeRoot = "api/external/grpc/client/billing"
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Exports:    map[string]project.Export{"billing": {Kind: "grpc", Path: treeRoot}},
		Components: project.Components{GRPC: &project.GRPC{Server: &project.GRPCServer{ProtoRoot: treeRoot}}},
	}}}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", treeRoot+"/.devctl-contract.json").Return(contract.File{
		Content: []byte(`{"kind":"grpc","format":"proto","module_root":"api/proto/grpc","buf_config":"buf.yaml"}`),
	}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", treeRoot+"/buf.yaml").Return(contract.File{}, nil)
	reader.EXPECT().ReadTree(gomock.Any(), "/checkout", treeRoot+"/api/proto/grpc").Return([]contract.File{{
		Path: treeRoot + "/api/proto/grpc/service.proto",
	}}, nil)
	reader.EXPECT().ReadTree(gomock.Any(), "/checkout", treeRoot).Return([]contract.File{
		{Path: treeRoot + "/.devctl-contract.json", Content: []byte(`{}`), Mode: 0o644},
		{Path: treeRoot + "/api/proto/grpc/service.proto", Content: []byte("syntax = \"proto3\";\n"), Mode: 0o644},
		{Path: treeRoot + "/buf.yaml", Content: []byte("version: v2\n"), Mode: 0o644},
	}, nil)

	snapshot, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{Export: "billing", Format: "proto"},
	})

	require.NoError(t, err)
	require.Equal(t, "api/proto/grpc", snapshot.ModuleRoot)
	require.Empty(t, snapshot.Entrypoint)
	require.Equal(t, []string{"api/proto/grpc/service.proto", "buf.yaml"}, snapshotPaths(snapshot))
}

func TestDevctlDoesNotFallbackWhenCommittedGRPCMetadataIsInvalid(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Exports: map[string]project.Export{"billing": {Kind: "grpc", Path: "api/proto/grpc"}},
		Components: project.Components{GRPC: &project.GRPC{Server: &project.GRPCServer{
			ProtoRoot: "api/proto/grpc",
		}}},
	}}}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", "api/proto/grpc/.devctl-contract.json").Return(contract.File{
		Content: []byte(`{"kind":"grpc","format":"proto","buf_config":"buf.yaml"}`),
	}, nil)

	_, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{Export: "billing", Format: "proto"},
	})

	var metadataErr *contract.SnapshotMetadataError
	require.ErrorAs(t, err, &metadataErr)
	require.Equal(t, "module_root", metadataErr.Field)
	require.Equal(t, contract.MetadataRequired, metadataErr.Reason)
}

func TestDevctlReportsMissingNamedExport(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{Exports: map[string]project.Export{}}}}, nil)

	_, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{Export: "missing"},
	})

	require.Equal(t, failure.NotFound, failure.CategoryOf(err))
}

func TestDevctlRejectsKafkaExportTopicMismatch(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Exports: map[string]project.Export{"events": {Kind: "kafka", Producer: "events"}},
		Components: project.Components{Kafka: &project.Kafka{Producers: []project.KafkaProducer{{
			Name: "events", Topic: "upstream_service.domain.events.v1", Contract: project.KafkaContract{Format: "raw"},
		}}}},
	}}}, nil)

	_, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source:    project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{Export: "events", Topic: "downstream_service.domain.events.v1"},
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
}

func TestDevctlRejectsKafkaExportFormatMismatch(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Exports: map[string]project.Export{"events": {Kind: "kafka", Producer: "events"}},
		Components: project.Components{Kafka: &project.Kafka{Producers: []project.KafkaProducer{{
			Name: "events", Topic: "upstream_service.domain.events.v1", Contract: project.KafkaContract{Format: "raw"},
		}}}},
	}}}, nil)

	_, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source: project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{
			Export: "events", Topic: "upstream_service.domain.events.v1", Format: "json",
		},
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
}

func TestDevctlMaterializesKafkaExportContractAndMetadata(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Sources: map[string]project.Source{"contracts": {Type: project.SourceLocal, Path: "api/contracts"}},
		Exports: map[string]project.Export{"events": {Kind: "kafka", Producer: "events"}},
		Components: project.Components{Kafka: &project.Kafka{Producers: []project.KafkaProducer{{
			Name: "events", Topic: "upstream_service.domain.events.v1", Contract: project.KafkaContract{
				Format: "json", Source: "contracts", Path: "schemas/events.json",
			},
		}}}},
	}}}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout/api/contracts", "schemas/events.json").Return(contract.File{
		Content: []byte(`{"type":"object"}`), Mode: 0o644,
	}, nil)

	snapshot, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source: project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{
			Export: "events", Topic: "upstream_service.domain.events.v1", Format: "json",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "schemas/events.json", snapshot.Entrypoint)
	require.Equal(t, &contract.Metadata{
		Kind: "kafka", Topic: "upstream_service.domain.events.v1", Format: "json", Entrypoint: "schemas/events.json",
	}, snapshot.Metadata)
	require.Equal(t, []string{"schemas/events.json"}, snapshotPaths(snapshot))
}

func TestDevctlMaterializesProtoKafkaExportMetadataAndBufFiles(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Sources: map[string]project.Source{"contracts": {
			Type: project.SourceLocal, Path: "api/contracts", Proto: project.SourceProto{BufConfig: "buf.yaml"},
		}},
		Exports: map[string]project.Export{"events": {Kind: "kafka", Producer: "events"}},
		Components: project.Components{Kafka: &project.Kafka{Producers: []project.KafkaProducer{{
			Name: "events", Topic: "upstream_service.domain.events.v1", Contract: project.KafkaContract{
				Format: "proto", Source: "contracts", Path: "proto/events.proto", ProtoRoot: "proto",
			},
		}}}},
	}}}, nil)
	reader.EXPECT().ReadTree(gomock.Any(), "/checkout/api/contracts", "proto").Return([]contract.File{{
		Path: "proto/events.proto", Content: []byte("syntax = \"proto3\";\n"), Mode: 0o644,
	}}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout/api/contracts", "buf.yaml").Return(contract.File{
		Content: []byte("version: v2\n"), Mode: 0o644,
	}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout/api/contracts", "buf.lock").Return(contract.File{
		Content: []byte("deps: []\n"), Mode: 0o644,
	}, nil)

	snapshot, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source: project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{
			Export: "events", Topic: "upstream_service.domain.events.v1", Format: "proto",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "proto", snapshot.ModuleRoot)
	require.Equal(t, "proto/events.proto", snapshot.Entrypoint)
	require.Equal(t, &contract.Metadata{
		Kind: "kafka", Topic: "upstream_service.domain.events.v1", Format: "proto",
		Entrypoint: "proto/events.proto", ModuleRoot: "proto", BufConfig: "buf.yaml",
	}, snapshot.Metadata)
	require.Equal(t, []string{"buf.lock", "buf.yaml", "proto/events.proto"}, snapshotPaths(snapshot))
}

func TestDevctlMaterializesKafkaExportFromUpstreamSyncedTree(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Paths:   project.ManifestPaths{ExternalContracts: "api/external"},
		Sources: map[string]project.Source{"contracts": {Type: project.SourceDevctl, Repo: "contracts", Ref: "v1"}},
		Exports: map[string]project.Export{"events": {Kind: "kafka", Producer: "events"}},
		Components: project.Components{Kafka: &project.Kafka{Producers: []project.KafkaProducer{{
			Name: "events", Topic: "upstream_service.domain.events.v1", Contract: project.KafkaContract{
				Format: "json", Source: "contracts", Export: "upstream-events",
			},
		}}}},
	}}}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", "api/external/kafka/producer/events/.devctl-contract.json").Return(contract.File{
		Content: []byte(`{"kind":"kafka","topic":"upstream_service.domain.events.v1","format":"json","entrypoint":"schemas/events.json"}`),
	}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", "api/external/kafka/producer/events/schemas/events.json").Return(
		contract.File{}, nil,
	)
	reader.EXPECT().ReadTree(gomock.Any(), "/checkout", "api/external/kafka/producer/events").Return([]contract.File{
		{Path: "api/external/kafka/producer/events/.devctl-contract.json", Content: []byte(`{}`), Mode: 0o644},
		{Path: "api/external/kafka/producer/events/schemas/events.json", Content: []byte(`{"type":"object"}`), Mode: 0o644},
	}, nil)

	snapshot, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source: project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{
			Export: "events", Topic: "upstream_service.domain.events.v1", Format: "json",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "schemas/events.json", snapshot.Entrypoint)
	require.Equal(t, &contract.Metadata{
		Kind: "kafka", Topic: "upstream_service.domain.events.v1", Format: "json", Entrypoint: "schemas/events.json",
	}, snapshot.Metadata)
	require.Equal(t, []string{"schemas/events.json"}, snapshotPaths(snapshot))
}

func TestDevctlReexportsCommittedProtoKafkaSnapshotOffline(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockGitClient(ctrl)
	manifests := mocks.NewMockManifestRepository(ctrl)
	reader := mocks.NewMockFileReader(ctrl)
	client.EXPECT().WithCheckout(gomock.Any(), "repo", "v1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, use func(string) error) error { return use("/checkout") },
	)
	const topic = "upstream_service.domain.events.v1"
	manifests.EXPECT().Load(gomock.Any(), "/checkout/devctl.yaml").Return(project.LoadManifestResult{Project: project.Project{Manifest: project.Manifest{
		Sources: map[string]project.Source{"contracts": {Type: project.SourceDevctl, Repo: "unavailable", Ref: "v1"}},
		Exports: map[string]project.Export{"events": {Kind: "kafka", Producer: "events"}},
		Components: project.Components{Kafka: &project.Kafka{Producers: []project.KafkaProducer{{
			Name: "events", Topic: topic, Contract: project.KafkaContract{
				Format: "proto", Source: "contracts", Export: "upstream-events",
			},
		}}}},
	}}}, nil)
	const treeRoot = "api/external/kafka/producer/events"
	metadata := `{"kind":"kafka","topic":"upstream_service.domain.events.v1","format":"proto","entrypoint":"proto/events.proto","module_root":"proto","buf_config":"buf.yaml"}`
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", treeRoot+"/.devctl-contract.json").Return(contract.File{
		Content: []byte(metadata),
	}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", treeRoot+"/proto/events.proto").Return(contract.File{}, nil)
	reader.EXPECT().ReadFile(gomock.Any(), "/checkout", treeRoot+"/buf.yaml").Return(contract.File{}, nil)
	reader.EXPECT().ReadTree(gomock.Any(), "/checkout", treeRoot+"/proto").Return([]contract.File{{
		Path: treeRoot + "/proto/events.proto",
	}}, nil)
	reader.EXPECT().ReadTree(gomock.Any(), "/checkout", treeRoot).Return([]contract.File{
		{Path: treeRoot + "/.devctl-contract.json", Content: []byte(metadata), Mode: 0o644},
		{Path: treeRoot + "/proto/events.proto", Content: []byte("syntax = \"proto3\";\n"), Mode: 0o644},
		{Path: treeRoot + "/buf.yaml", Content: []byte("version: v2\n"), Mode: 0o644},
		{Path: treeRoot + "/buf.lock", Content: []byte("deps: []\n"), Mode: 0o644},
	}, nil)

	snapshot, err := materialize.NewDevctl(client, manifests, reader).Materialize(context.Background(), materializedomain.Request{
		Source: project.Source{Type: project.SourceDevctl, Repo: "repo", Ref: "v1"},
		Reference: contract.Reference{
			Export: "events", Topic: topic, Format: "proto",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "proto", snapshot.ModuleRoot)
	require.Equal(t, "proto/events.proto", snapshot.Entrypoint)
	require.Equal(t, []string{"buf.lock", "buf.yaml", "proto/events.proto"}, snapshotPaths(snapshot))
	require.Equal(t, "buf.yaml", snapshot.Metadata.BufConfig)
}
