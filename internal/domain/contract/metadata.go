package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/failure"
)

// MetadataInvalidReason identifies why committed Snapshot Metadata is stale.
type MetadataInvalidReason string

const (
	MetadataRequired    MetadataInvalidReason = "required"
	MetadataInvalidType MetadataInvalidReason = "invalid_type"
	MetadataInvalidPath MetadataInvalidReason = "invalid_path"
	MetadataMismatch    MetadataInvalidReason = "mismatch"
	MetadataNotFound    MetadataInvalidReason = "not_found"
	MetadataNotRegular  MetadataInvalidReason = "not_regular"
	MetadataUnexpected  MetadataInvalidReason = "unexpected"
)

// SnapshotMetadataError reports stale committed metadata and tells callers how to refresh it.
type SnapshotMetadataError struct {
	Field  string
	Reason MetadataInvalidReason
	Hint   string
	Cause  error
}

func (e *SnapshotMetadataError) Error() string {
	return fmt.Sprintf("snapshot metadata field %q is invalid: %s", e.Field, e.Reason)
}

func (e *SnapshotMetadataError) Unwrap() error { return e.Cause }

func (e *SnapshotMetadataError) Category() failure.Category { return failure.InvalidInput }

// MetadataExpectation binds a committed Snapshot to its consuming Target.
type MetadataExpectation struct {
	Kind   string
	Topic  string
	Format string
}

// DecodeMetadata decodes one strict sidecar document.
func DecodeMetadata(data []byte) (Metadata, error) {
	var metadata Metadata
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return Metadata{}, invalidMetadata(jsonErrorField(err), MetadataInvalidType, err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return Metadata{}, invalidMetadata(".devctl-contract.json", MetadataInvalidType, err)
	}
	return metadata, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return fmt.Errorf("decoder.Decode: %w", err)
}

func jsonErrorField(err error) string {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		parts := strings.Split(typeErr.Field, ".")
		return jsonFieldName(parts[len(parts)-1])
	}
	const unknownPrefix = "json: unknown field \""
	if message := err.Error(); strings.HasPrefix(message, unknownPrefix) && strings.HasSuffix(message, "\"") {
		return strings.TrimSuffix(strings.TrimPrefix(message, unknownPrefix), "\"")
	}
	return ".devctl-contract.json"
}

func jsonFieldName(name string) string {
	switch name {
	case "Kind":
		return "kind"
	case "Topic":
		return "topic"
	case "Format":
		return "format"
	case "Entrypoint":
		return "entrypoint"
	case "ModuleRoot":
		return "module_root"
	case "BufConfig":
		return "buf_config"
	default:
		return name
	}
}

// ValidateSnapshot checks metadata shape, Target identity, paths, and referenced files.
func ValidateSnapshot(snapshot Snapshot, expected MetadataExpectation) error {
	if snapshot.Metadata == nil {
		return invalidMetadata(".devctl-contract.json", MetadataRequired, nil)
	}
	metadata := *snapshot.Metadata
	if err := ValidateMetadata(metadata, expected); err != nil {
		return err
	}
	files := make(map[string]struct{}, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[path.Clean(file.Path)] = struct{}{}
	}
	if metadata.Entrypoint != "" {
		if _, exists := files[metadata.Entrypoint]; !exists {
			return invalidMetadata("entrypoint", MetadataNotFound, nil)
		}
	}
	if metadata.BufConfig != "" {
		if _, exists := files[metadata.BufConfig]; !exists {
			return invalidMetadata("buf_config", MetadataNotFound, nil)
		}
	}
	if metadata.ModuleRoot != "" && !containsModuleFile(files, metadata.ModuleRoot) {
		return invalidMetadata("module_root", MetadataNotFound, nil)
	}
	if metadata.Format == "raw" && len(snapshot.Files) != 0 {
		return invalidMetadata("files", MetadataUnexpected, nil)
	}
	return nil
}

// ValidateMetadata checks one sidecar's shape and consuming Target identity.
func ValidateMetadata(metadata Metadata, expected MetadataExpectation) error {
	if err := validateIdentity(metadata, expected); err != nil {
		return err
	}
	return validateMetadataShape(metadata)
}

func validateIdentity(metadata Metadata, expected MetadataExpectation) error {
	for _, value := range []struct {
		field, actual, expected string
	}{
		{"kind", metadata.Kind, expected.Kind},
		{"topic", metadata.Topic, expected.Topic},
		{"format", metadata.Format, expected.Format},
	} {
		if value.expected != "" && value.actual != value.expected {
			return invalidMetadata(value.field, MetadataMismatch, nil)
		}
	}
	return nil
}

func validateMetadataShape(metadata Metadata) error {
	if metadata.Kind == "" {
		return invalidMetadata("kind", MetadataRequired, nil)
	}
	switch metadata.Kind {
	case "grpc":
		if metadata.Format != "proto" {
			return invalidMetadata("format", MetadataMismatch, nil)
		}
		if metadata.Topic != "" {
			return invalidMetadata("topic", MetadataUnexpected, nil)
		}
		if metadata.Entrypoint != "" {
			return invalidMetadata("entrypoint", MetadataUnexpected, nil)
		}
		return validateProtoMetadata(metadata, false)
	case "kafka":
		return validateKafkaMetadata(metadata)
	default:
		return invalidMetadata("kind", MetadataMismatch, nil)
	}
}

func validateKafkaMetadata(metadata Metadata) error {
	if metadata.Topic == "" {
		return invalidMetadata("topic", MetadataRequired, nil)
	}
	switch metadata.Format {
	case "raw":
		for _, value := range []struct{ field, value string }{
			{"entrypoint", metadata.Entrypoint}, {"module_root", metadata.ModuleRoot}, {"buf_config", metadata.BufConfig},
		} {
			if value.value != "" {
				return invalidMetadata(value.field, MetadataUnexpected, nil)
			}
		}
		return nil
	case "json":
		if err := requireMetadataPath("entrypoint", metadata.Entrypoint, false); err != nil {
			return err
		}
		if metadata.ModuleRoot != "" {
			return invalidMetadata("module_root", MetadataUnexpected, nil)
		}
		if metadata.BufConfig != "" {
			return invalidMetadata("buf_config", MetadataUnexpected, nil)
		}
		return nil
	case "proto":
		return validateProtoMetadata(metadata, true)
	default:
		return invalidMetadata("format", MetadataMismatch, nil)
	}
}

func validateProtoMetadata(metadata Metadata, requireEntrypoint bool) error {
	if err := requireMetadataPath("module_root", metadata.ModuleRoot, true); err != nil {
		return err
	}
	if err := requireMetadataPath("buf_config", metadata.BufConfig, false); err != nil {
		return err
	}
	if requireEntrypoint {
		if err := requireMetadataPath("entrypoint", metadata.Entrypoint, false); err != nil {
			return err
		}
		if !pathWithin(metadata.ModuleRoot, metadata.Entrypoint) {
			return invalidMetadata("entrypoint", MetadataInvalidPath, nil)
		}
	}
	return nil
}

func requireMetadataPath(field, value string, allowCurrent bool) error {
	if value == "" {
		return invalidMetadata(field, MetadataRequired, nil)
	}
	if allowCurrent && value == "." {
		return nil
	}
	if !safeMetadataPath(value) {
		return invalidMetadata(field, MetadataInvalidPath, nil)
	}
	return nil
}

func safeMetadataPath(value string) bool {
	return value != "" && value != "." && !path.IsAbs(value) &&
		path.Clean(value) == value && value != ".." && !strings.HasPrefix(value, "../") &&
		!strings.Contains(value, "\\")
}

func pathWithin(root, name string) bool {
	return root == "." || name == root || strings.HasPrefix(name, root+"/")
}

func containsModuleFile(files map[string]struct{}, moduleRoot string) bool {
	for name := range files {
		if pathWithin(moduleRoot, name) {
			return true
		}
	}
	return false
}

func invalidMetadata(field string, reason MetadataInvalidReason, cause error) error {
	return &SnapshotMetadataError{Field: field, Reason: reason, Hint: "devctl sync", Cause: cause}
}
