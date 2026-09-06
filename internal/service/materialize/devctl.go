package materialize

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"

	"github.com/devctllabs/devctl/internal/domain/contract"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	"github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/contractsnapshot"
)

const contractMetadataFile = ".devctl-contract.json"

// DevctlService resolves a named export from a temporary upstream Devctl project checkout.
type DevctlService struct {
	client    GitClient
	manifests ManifestRepository
	reader    FileReader
	snapshots *contractsnapshot.Loader
}

func NewDevctl(client GitClient, manifests ManifestRepository, reader FileReader) *DevctlService {
	return &DevctlService{
		client: client, manifests: manifests, reader: reader,
		snapshots: contractsnapshot.New(reader),
	}
}

func (s *DevctlService) SourceType() project.SourceType { return project.SourceDevctl }

func (s *DevctlService) Materialize(ctx context.Context, request materializedomain.Request) (contract.Snapshot, error) {
	var snapshot contract.Snapshot
	err := s.client.WithCheckout(ctx, request.Source.Repo, request.Source.Ref, func(root string) error {
		var materializeErr error
		snapshot, materializeErr = s.materializeCheckout(ctx, root, request)
		return materializeErr
	})
	if err != nil {
		return snapshot, fmt.Errorf("client.WithCheckout: %w", err)
	}
	return snapshot, nil
}

func (s *DevctlService) materializeCheckout(ctx context.Context, root string, request materializedomain.Request) (contract.Snapshot, error) {
	loaded, err := s.manifests.Load(ctx, filepath.Join(root, "devctl.yaml"))
	if err != nil {
		return contract.Snapshot{}, &materializedomain.UpstreamManifestError{Repository: request.Source.Repo, Ref: request.Source.Ref, Cause: err}
	}
	if len(loaded.Issues) > 0 {
		return contract.Snapshot{}, &materializedomain.UpstreamManifestError{Repository: request.Source.Repo, Ref: request.Source.Ref}
	}
	manifest := loaded.Project.Manifest
	exported, exists := manifest.Exports[request.Reference.Export]
	if !exists {
		return contract.Snapshot{}, &materializedomain.ExportNotFoundError{Name: request.Reference.Export}
	}
	if !manifest.ExportMatchesSurface(exported) {
		return contract.Snapshot{}, &materializedomain.InvalidExportError{Name: request.Reference.Export}
	}
	switch exported.Kind {
	case "openapi":
		return referenceClosure(ctx, s.reader, root, exported.Path)
	case "grpc":
		return s.materializeGRPCExport(ctx, root, manifest, exported)
	case "kafka":
		producer, exists := kafkaProducer(manifest, exported.Producer)
		if !exists {
			return contract.Snapshot{}, &materializedomain.ExportNotFoundError{Name: exported.Producer}
		}
		if request.Reference.Topic != "" && request.Reference.Topic != producer.Topic {
			return contract.Snapshot{}, &materializedomain.KafkaTopicMismatchError{Expected: producer.Topic, Actual: request.Reference.Topic}
		}
		format := kafkaFormat(producer.Contract)
		if request.Reference.Format != "" && request.Reference.Format != format {
			return contract.Snapshot{}, &materializedomain.KafkaFormatMismatchError{Expected: format, Actual: request.Reference.Format}
		}
		return s.materializeKafkaProducer(ctx, root, manifest, producer)
	default:
		return contract.Snapshot{}, &materializedomain.UnsupportedExportError{Name: request.Reference.Export, Kind: exported.Kind}
	}
}

func (s *DevctlService) materializeGRPCExport(
	ctx context.Context,
	root string,
	manifest project.Manifest,
	exported project.Export,
) (contract.Snapshot, error) {
	snapshot, err := s.snapshots.Load(
		ctx, root, exported.Path, contract.MetadataExpectation{Kind: "grpc", Format: "proto"},
	)
	if err == nil {
		return snapshot, nil
	}
	if !committedMetadataRequired(err) {
		return contract.Snapshot{}, fmt.Errorf("snapshots.Load: %w", err)
	}
	files, err := s.reader.ReadTree(ctx, root, exported.Path)
	if err != nil {
		return contract.Snapshot{}, &materializedomain.OperationError{Operation: materializedomain.OperationReadFile, Path: exported.Path, Kind: materializedomain.FailureUnavailable, Cause: err}
	}
	if len(files) == 0 {
		return contract.Snapshot{}, &materializedomain.OperationError{Operation: materializedomain.OperationBuildSnapshot, Path: exported.Path, Kind: materializedomain.FailureNotFound}
	}
	bufConfig := "buf.yaml"
	if manifest.Components.GRPC.Server.BufConfig != "" {
		bufConfig = manifest.Components.GRPC.Server.BufConfig
	}
	files, err = includeBufFiles(ctx, s.reader, bufFilesRequest{
		root: root, files: files, configPath: bufConfig,
	})
	if err != nil {
		return contract.Snapshot{}, err
	}
	snapshot, err = newProtoSnapshot(exported.Path, "", files)
	if err != nil {
		return contract.Snapshot{}, err
	}
	snapshot.Metadata = &contract.Metadata{
		Kind: "grpc", Format: "proto", ModuleRoot: exported.Path, BufConfig: bufConfig,
	}
	return snapshot, nil
}

func (s *DevctlService) materializeKafkaProducer(ctx context.Context, root string, manifest project.Manifest, producer project.KafkaProducer) (contract.Snapshot, error) {
	format := kafkaFormat(producer.Contract)
	metadata := &contract.Metadata{Kind: "kafka", Topic: producer.Topic, Format: format}
	if format == "raw" {
		return contract.Snapshot{Metadata: metadata}, nil
	}
	source, exists := manifest.Sources[producer.Contract.Source]
	if !exists {
		return contract.Snapshot{}, &materializedomain.ExportNotFoundError{Name: producer.Contract.Source}
	}
	if source.Type != project.SourceLocal {
		return s.materializeSyncedKafkaProducer(ctx, root, manifest, producer)
	}
	sourceRoot := filepath.Join(root, filepath.FromSlash(source.Path))
	reference := contract.Reference{
		Entrypoint: producer.Contract.Path,
		Format:     format,
		ProtoRoot:  producer.Contract.ProtoRoot,
	}
	var snapshot contract.Snapshot
	var err error
	if format == "proto" {
		snapshot, err = protoTree(ctx, s.reader, protoTreeRequest{
			root: sourceRoot, reference: reference, bufConfig: source.Proto.BufConfig,
		})
	} else {
		snapshot, err = referenceClosure(ctx, s.reader, sourceRoot, reference.Entrypoint)
	}
	if err != nil {
		return contract.Snapshot{}, err
	}
	metadata.Entrypoint = snapshot.Entrypoint
	if format == "proto" {
		metadata.ModuleRoot = snapshot.ModuleRoot
		metadata.BufConfig = source.Proto.BufConfig
	}
	snapshot.Metadata = metadata
	if err := contract.ValidateSnapshot(snapshot, contract.MetadataExpectation{
		Kind: "kafka", Topic: producer.Topic, Format: format,
	}); err != nil {
		return contract.Snapshot{}, fmt.Errorf("contract.ValidateSnapshot: %w", err)
	}
	return snapshot, nil
}

func (s *DevctlService) materializeSyncedKafkaProducer(ctx context.Context, root string, manifest project.Manifest, producer project.KafkaProducer) (contract.Snapshot, error) {
	externalRoot := manifest.Paths.ExternalContracts
	if externalRoot == "" {
		externalRoot = "api/external"
	}
	treeRoot := path.Join(externalRoot, "kafka", "producer", producer.Name)
	snapshot, err := s.snapshots.Load(
		ctx, root, treeRoot,
		contract.MetadataExpectation{
			Kind: "kafka", Topic: producer.Topic, Format: kafkaFormat(producer.Contract),
		},
	)
	if err != nil {
		return contract.Snapshot{}, fmt.Errorf("snapshots.Load: %w", err)
	}
	return snapshot, nil
}

func committedMetadataRequired(err error) bool {
	var metadataErr *contract.SnapshotMetadataError
	return errors.As(err, &metadataErr) &&
		metadataErr.Field == contractMetadataFile && metadataErr.Reason == contract.MetadataRequired
}

func kafkaFormat(selected project.KafkaContract) string {
	if selected.Format == "" {
		return "raw"
	}
	return selected.Format
}

func kafkaProducer(manifest project.Manifest, name string) (project.KafkaProducer, bool) {
	if manifest.Components.Kafka == nil {
		return project.KafkaProducer{}, false
	}
	for _, producer := range manifest.Components.Kafka.Producers {
		if producer.Name == name {
			return producer, true
		}
	}
	return project.KafkaProducer{}, false
}
