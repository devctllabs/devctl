package projectreadiness

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/devctllabs/devctl/internal/domain/project"
	"golang.org/x/mod/modfile"
)

var environmentKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

const (
	oapiCodegenToolPath = "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
	bufToolPath         = "github.com/bufbuild/buf/cmd/buf"
)

//go:generate go tool mockgen -destination mocks/checker.go -package mocks -typed . Workspace

// Workspace exposes only filesystem facts used by Project Readiness policy.
type Workspace interface {
	// RegularFile reports whether relativePath is a regular non-symlink file below root.
	RegularFile(ctx context.Context, root, relativePath string) (bool, error)
	// Directory reports whether relativePath is a real directory below root.
	Directory(ctx context.Context, root, relativePath string) (bool, error)
	// ReadBytes reads the regular project file at relativePath below root.
	ReadBytes(ctx context.Context, root, relativePath string) ([]byte, error)
}

// Checker evaluates filesystem and tool readiness for a semantically inspected Project.
type Checker struct {
	workspace Workspace
}

func New(workspace Workspace) *Checker {
	return &Checker{workspace: workspace}
}

// Check returns every applicable readiness issue in stable order.
func (c *Checker) Check(ctx context.Context, selected project.Project) ([]project.Issue, error) {
	check := readiness{workspace: c.workspace, project: selected}
	goModExists, err := check.requireRegular(ctx, "go.mod", project.IssueGoModMissing)
	if err != nil {
		return nil, err
	}
	if err := check.sources(ctx); err != nil {
		return check.issues, err
	}
	if err := check.components(ctx, goModExists); err != nil {
		return check.issues, err
	}
	if err := check.migrations(ctx); err != nil {
		return check.issues, err
	}
	return check.issues, nil
}

func (r *readiness) components(ctx context.Context, goModExists bool) error {
	if r.project.Manifest.Components.HTTP != nil {
		if err := r.http(ctx, goModExists); err != nil {
			return err
		}
	}
	if r.project.Manifest.Components.GRPC != nil {
		if err := r.grpc(ctx, goModExists); err != nil {
			return err
		}
	}
	if r.project.Manifest.Components.Kafka != nil {
		if err := r.kafka(ctx, goModExists); err != nil {
			return err
		}
	}
	return nil
}

func (r *readiness) kafka(ctx context.Context, goModExists bool) error {
	manifest := r.project.Manifest
	if hasReadyKafkaContractFormat(manifest, "proto") {
		configs := make(map[string]struct{})
		for _, target := range project.NewTargetCatalog(manifest).Select(project.TargetOperationGenerate, "kafka", "") {
			if target.Format == "proto" {
				configs[target.Config] = struct{}{}
			}
		}
		for _, config := range sortedKeys(configs) {
			if _, err := r.requireRegular(ctx, config, project.IssueToolConfigMissing); err != nil {
				return err
			}
		}
		if goModExists {
			if err := r.goTool(ctx, bufToolPath); err != nil {
				return err
			}
		}
	}
	if hasReadyKafkaContractFormat(manifest, "json") {
		return r.miseTools(ctx, "node", "npm:quicktype")
	}
	return nil
}

func (r *readiness) miseTools(ctx context.Context, tools ...string) error {
	exists, err := r.requireRegular(ctx, ".mise.toml", project.IssueToolConfigMissing)
	if err != nil || !exists {
		return err
	}
	content, err := r.workspace.ReadBytes(ctx, r.project.Root, ".mise.toml")
	if err != nil {
		return fmt.Errorf("workspace.ReadBytes: %w", operationError(project.OperationReadFile, ".mise.toml", err))
	}
	configured, valid := decodeMiseTools(content)
	if !valid {
		r.add(project.IssueToolConfigInvalid, ".mise.toml")
		return nil
	}
	for _, tool := range tools {
		if _, exists := configured[tool]; !exists {
			r.issues = append(r.issues, project.Issue{
				Code: project.IssueToolMissing, Path: r.project.ManifestPath, Field: ".mise.toml",
				Parameters: &project.Parameters{Value: tool},
			})
		}
	}
	return nil
}

func decodeMiseTools(content []byte) (map[string]toml.Primitive, bool) {
	var config struct {
		Tools map[string]toml.Primitive `toml:"tools"`
	}
	_, err := toml.Decode(string(content), &config)
	return config.Tools, err == nil
}

func hasReadyKafkaContractFormat(manifest project.Manifest, format string) bool {
	kafka := manifest.Components.Kafka
	if kafka == nil {
		return false
	}
	for _, consumer := range kafka.Consumers {
		if kafkaContractReady(manifest, consumer.Contract, format) {
			return true
		}
	}
	for _, producer := range kafka.Producers {
		if kafkaContractReady(manifest, producer.Contract, format) {
			return true
		}
	}
	return false
}

func kafkaContractReady(manifest project.Manifest, selected project.KafkaContract, format string) bool {
	if selected.Format != format || selected.Source == "" {
		return false
	}
	_, exists := manifest.Sources[selected.Source]
	return exists
}

func (r *readiness) grpc(ctx context.Context, goModExists bool) error {
	manifest := r.project.Manifest
	grpc := manifest.Components.GRPC
	if grpc.Server == nil && !hasReadyGRPCClient(manifest, grpc.Clients) {
		return nil
	}
	if grpc.Server != nil {
		config := grpc.Server.BufConfig
		if config == "" {
			config = "buf.yaml"
		}
		if _, err := r.requireRegular(ctx, config, project.IssueToolConfigMissing); err != nil {
			return err
		}
	}
	configs := make(map[string]struct{})
	for _, target := range project.NewTargetCatalog(manifest).Select(project.TargetOperationGenerate, "grpc", "") {
		if safeRelative(target.Config) {
			configs[target.Config] = struct{}{}
		}
	}
	for _, config := range sortedKeys(configs) {
		if _, err := r.requireRegular(ctx, config, project.IssueToolConfigMissing); err != nil {
			return err
		}
	}
	if goModExists {
		return r.goTool(ctx, bufToolPath)
	}
	return nil
}

func hasReadyGRPCClient(manifest project.Manifest, clients []project.GRPCClient) bool {
	for _, client := range clients {
		if _, exists := manifest.Sources[client.Source]; exists {
			return true
		}
	}
	return false
}

func (r *readiness) http(ctx context.Context, goModExists bool) error {
	manifest := r.project.Manifest
	targets := project.NewTargetCatalog(manifest).Select(project.TargetOperationGenerate, "http", "")
	for _, target := range targets {
		if target.Role == "server" && safeRelative(target.Reference.Entrypoint) {
			if _, err := r.requireRegular(ctx, target.Reference.Entrypoint, project.IssueOpenAPIMissing); err != nil {
				return err
			}
		}
	}
	if manifest.Languages.Go.Generators.HTTP == nil {
		r.add(project.IssueHTTPGeneratorMissing, "languages.go.generators.http")
	} else {
		configs := make(map[string]struct{}, len(targets))
		for _, target := range targets {
			configs[target.Config] = struct{}{}
		}
		for _, config := range sortedKeys(configs) {
			if _, err := r.requireRegular(ctx, config, project.IssueToolConfigMissing); err != nil {
				return err
			}
		}
	}
	if goModExists {
		return r.goTool(ctx, oapiCodegenToolPath)
	}
	return nil
}

func (r *readiness) goTool(ctx context.Context, toolPath string) error {
	if _, checked := r.tools[toolPath]; checked {
		return nil
	}
	if r.tools == nil {
		r.tools = make(map[string]struct{})
	}
	r.tools[toolPath] = struct{}{}
	content, err := r.workspace.ReadBytes(ctx, r.project.Root, "go.mod")
	if err != nil {
		return fmt.Errorf("workspace.ReadBytes: %w", operationError(project.OperationReadFile, "go.mod", err))
	}
	parsed, valid := parseGoMod(content)
	if !valid {
		r.add(project.IssueGoModInvalid, "go.mod")
		return nil
	}
	for _, tool := range parsed.Tool {
		if tool.Path == toolPath {
			return nil
		}
	}
	r.add(project.IssueToolMissing, "go.mod")
	return nil
}

func parseGoMod(content []byte) (*modfile.File, bool) {
	parsed, err := modfile.Parse("go.mod", content, nil)
	return parsed, err == nil
}

type readiness struct {
	workspace Workspace
	project   project.Project
	issues    []project.Issue
	tools     map[string]struct{}
}

func (r *readiness) sources(ctx context.Context) error {
	for _, name := range sortedKeys(r.project.Manifest.Sources) {
		source := r.project.Manifest.Sources[name]
		if source.Type != project.SourceLocal || !safeRelative(source.Path) {
			continue
		}
		exists, err := r.directory(ctx, source.Path)
		if err != nil {
			return err
		}
		if !exists {
			r.add(project.IssueSourceMissing, source.Path)
			continue
		}
		if source.Proto.BufConfig != "" && safeRelative(source.Proto.BufConfig) {
			if _, err := r.requireRegular(ctx, path.Join(source.Path, source.Proto.BufConfig), project.IssueToolConfigMissing); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *readiness) migrations(ctx context.Context) error {
	if r.project.Manifest.Components.DB == nil {
		return nil
	}
	var paths []string
	for _, connection := range r.project.Manifest.Components.DB.Connections {
		for _, variant := range connection.Variants {
			if migrationsReady(variant.Kind, variant.Migrations) {
				paths = append(paths, variant.Migrations.Path)
			}
		}
	}
	sort.Strings(paths)
	for _, migrationPath := range paths {
		exists, err := r.directory(ctx, migrationPath)
		if err != nil {
			return err
		}
		if !exists {
			r.add(project.IssueMigrationPathMissing, migrationPath)
		}
	}
	return nil
}

func (r *readiness) directory(ctx context.Context, relativePath string) (bool, error) {
	exists, err := r.workspace.Directory(ctx, r.project.Root, relativePath)
	if err != nil {
		return false, fmt.Errorf("workspace.Directory: %w", operationError(project.OperationInspectFile, relativePath, err))
	}
	return exists, nil
}

func migrationsReady(kind string, migrations *project.DBMigrations) bool {
	if migrations == nil {
		return false
	}
	validKind := kind == "sqlite" || kind == "postgres" || kind == "clickhouse"
	if !validKind || !safeRelative(migrations.Path) || !environmentKey.MatchString(migrations.DatabaseEnv) {
		return false
	}
	if migrations.DatabaseDefault == "" {
		return true
	}
	parsed, err := url.Parse(migrations.DatabaseDefault)
	if err != nil {
		return false
	}
	switch kind {
	case "sqlite":
		return parsed.Scheme == "sqlite"
	case "clickhouse":
		return parsed.Scheme == "clickhouse"
	default:
		return parsed.Scheme == "postgres" || parsed.Scheme == "postgresql"
	}
}

func (r *readiness) requireRegular(ctx context.Context, relativePath string, code project.IssueCode) (bool, error) {
	exists, err := r.workspace.RegularFile(ctx, r.project.Root, relativePath)
	if err != nil {
		return false, fmt.Errorf("workspace.RegularFile: %w", operationError(project.OperationInspectFile, relativePath, err))
	}
	if !exists {
		r.add(code, relativePath)
	}
	return exists, nil
}

func operationError(operation project.Operation, selectedPath string, cause error) error {
	return &project.OperationError{
		Operation: operation,
		Path:      selectedPath,
		Kind:      project.FailureUnavailable,
		Cause:     cause,
	}
}

func (r *readiness) add(code project.IssueCode, field string) {
	r.issues = append(r.issues, project.Issue{Code: code, Path: r.project.ManifestPath, Field: field})
}

func safeRelative(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") {
		return false
	}
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func sortedKeys[Value any](values map[string]Value) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
