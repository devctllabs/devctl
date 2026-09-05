package lint

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/contract"
	lintdomain "github.com/devctllabs/devctl/internal/domain/lint"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	platformjsonschema "github.com/devctllabs/devctl/internal/platform/jsonschema"
	platformopenapi "github.com/devctllabs/devctl/internal/platform/openapi"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.uber.org/zap"
)

//go:generate go tool mockgen -destination mocks/service.go -package mocks -typed . ProjectRepository,ContractLocator,TargetResolver,ProtoLinter

// ProjectRepository resolves the valid project selected for linting.
type ProjectRepository interface {
	// LoadProject returns a structurally and semantically valid project or an execution error.
	LoadProject(ctx context.Context, manifestPath string) (projectdomain.Project, error)
}

// ContractLocator resolves and reads contained local or materialized contracts.
type ContractLocator interface {
	// ResolveContract returns the contained entrypoint selected by location.
	ResolveContract(ctx context.Context, location contract.Location) (string, error)
	// ReadContract returns the exact bytes at path without interpreting OpenAPI semantics.
	ReadContract(ctx context.Context, path string) ([]byte, error)
	// ListProtoFiles returns sorted project-relative Proto files below relativeRoot.
	ListProtoFiles(ctx context.Context, root, relativeRoot string) ([]string, error)
}

// TargetResolver attaches the concrete input required to lint one Target.
type TargetResolver interface {
	// Resolve attaches the concrete input required to execute target in selected Project.
	Resolve(ctx context.Context, selected projectdomain.Project, target projectdomain.Target) (projectdomain.Target, error)
}

// ProtoLinter checks one contained Proto target with project-pinned Buf.
type ProtoLinter interface {
	// Lint checks target content without modifying project files.
	Lint(ctx context.Context, project projectdomain.Project, target projectdomain.Target) error
}

type Service struct {
	logger    *zap.Logger
	projects  ProjectRepository
	contracts ContractLocator
	inputs    TargetResolver
	proto     ProtoLinter
}

// Dependencies names the required lint capabilities passed to New.
type Dependencies struct {
	Projects  ProjectRepository
	Contracts ContractLocator
	Inputs    TargetResolver
	Proto     ProtoLinter
}

var protoFilenamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z][a-z0-9]*)*\.[a-z][a-z0-9]*(?:_[a-z][a-z0-9]*)*\.proto$`)
var kafkaTopicPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z][a-z0-9]*)*\.[a-z][a-z0-9]*(?:_[a-z][a-z0-9]*)*\.[a-z][a-z0-9]*(?:_[a-z][a-z0-9]*)*\.v[1-9][0-9]*$`)

func New(logger *zap.Logger, dependencies Dependencies) *Service {
	return &Service{
		logger: logger, projects: dependencies.Projects, contracts: dependencies.Contracts,
		inputs: dependencies.Inputs, proto: dependencies.Proto,
	}
}

// Lint aggregates findings across configured contracts in deterministic order.
// Findings are normal results; an execution error returns findings collected from earlier contracts.
func (s *Service) Lint(ctx context.Context, command lintdomain.Command) (lintdomain.Result, error) {
	result := lintdomain.Result{Valid: true, Contracts: []string{}, Issues: []lintdomain.Issue{}}
	project, err := s.projects.LoadProject(ctx, command.ManifestPath)
	if err != nil {
		return result, fmt.Errorf("projects.LoadProject: %w", err)
	}
	targets, err := projectdomain.NewTargetCatalog(project.Manifest).Resolve(
		projectdomain.TargetOperationLint, command.Family, "",
	)
	if err != nil {
		return result, fmt.Errorf("catalog.Resolve: %w", err)
	}
	for _, target := range targets {
		result, err = s.lintTarget(ctx, project, result, target)
		if err != nil {
			return result, err
		}
	}
	result.Valid = len(result.Issues) == 0
	s.logger.Debug("contract lint completed", zap.Bool("valid", result.Valid), zap.Int("contracts", len(result.Contracts)))
	return result, nil
}

func (s *Service) lintTarget(
	ctx context.Context,
	project projectdomain.Project,
	result lintdomain.Result,
	target projectdomain.Target,
) (lintdomain.Result, error) {
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("ctx.Err: %w", err)
	}
	resolved, err := s.inputs.Resolve(ctx, project, target)
	if err != nil {
		if target.Family == "http" {
			operationErr := &lintdomain.OperationError{Operation: lintdomain.OperationLocateContract, Target: target.ID, Path: target.Reference.Entrypoint, Kind: lintdomain.FailureUnavailable, Cause: err}
			return result, fmt.Errorf("inputs.Resolve: %w", operationErr)
		}
		return result, fmt.Errorf("inputs.Resolve: %w", err)
	}
	target = resolved
	switch target.Family {
	case "http":
		return s.lintHTTPTarget(ctx, project, result, target)
	case "grpc":
		return s.lintGRPCTarget(ctx, project, result, target)
	case "kafka":
		return s.appendKafkaResult(ctx, project, result, target)
	default:
		return result, nil
	}
}

func (s *Service) lintHTTPTarget(
	ctx context.Context,
	project projectdomain.Project,
	result lintdomain.Result,
	target projectdomain.Target,
) (lintdomain.Result, error) {
	contractPath := target.Input
	result.Contracts = append(result.Contracts, target.ID)
	data, err := s.contracts.ReadContract(ctx, contractPath)
	if err != nil {
		operationErr := &lintdomain.OperationError{Operation: lintdomain.OperationReadContract, Target: target.ID, Path: contractPath, Kind: lintdomain.FailureUnavailable, Cause: err}
		return result, fmt.Errorf("contracts.ReadContract: %w", operationErr)
	}
	result.Issues = append(result.Issues, lintIssues(target, contractPath, platformopenapi.Analyze(data))...)
	return result, nil
}

func (s *Service) appendKafkaResult(ctx context.Context, project projectdomain.Project, result lintdomain.Result, target projectdomain.Target) (lintdomain.Result, error) {
	targetID := target.ID
	result.Contracts = append(result.Contracts, targetID)
	if !kafkaTopicPattern.MatchString(target.Reference.Topic) {
		result.Issues = append(result.Issues, lintdomain.Issue{Code: "kafka_topic", Target: targetID})
	}
	if localKafkaSchemaMismatch(target) {
		result.Issues = append(result.Issues, lintdomain.Issue{Code: "kafka_schema_filename", Target: targetID, Path: target.Reference.Entrypoint})
	}
	switch target.Format {
	case "json":
		return s.lintKafkaJSON(ctx, project, result, target)
	case "proto":
		return s.lintKafkaProto(ctx, project, result, target)
	default:
		return result, nil
	}
}

func localKafkaSchemaMismatch(target projectdomain.Target) bool {
	if target.Source.Type != projectdomain.SourceLocal || target.Reference.Entrypoint == "" || target.Format != "proto" && target.Format != "json" {
		return false
	}
	return path.Base(target.Reference.Entrypoint) != target.Reference.Topic+"."+target.Format
}

func (s *Service) lintKafkaJSON(ctx context.Context, project projectdomain.Project, result lintdomain.Result, target projectdomain.Target) (lintdomain.Result, error) {
	targetID := target.ID
	location := target.Location
	location.Root = project.Root
	contractPath, err := s.contracts.ResolveContract(ctx, location)
	if err != nil {
		operationErr := &lintdomain.OperationError{Operation: lintdomain.OperationLocateContract, Target: targetID, Path: target.Reference.Entrypoint, Kind: lintdomain.FailureUnavailable, Cause: err}
		return result, fmt.Errorf("contracts.ResolveContract: %w", operationErr)
	}
	data, err := s.contracts.ReadContract(ctx, contractPath)
	if err != nil {
		operationErr := &lintdomain.OperationError{Operation: lintdomain.OperationReadContract, Target: targetID, Path: contractPath, Kind: lintdomain.FailureUnavailable, Cause: err}
		return result, fmt.Errorf("contracts.ReadContract: %w", operationErr)
	}
	if jsonSchemaInvalid(data) {
		result.Issues = append(result.Issues, lintdomain.Issue{Code: "json_schema", Target: targetID, Path: contractPath})
		return result, nil
	}
	if jsonSchemaTitleMissing(data) {
		result.Issues = append(result.Issues, lintdomain.Issue{
			Code: "json_schema_title", Target: targetID, Path: contractPath, Field: "title",
		})
	}
	return result, nil
}

func jsonSchemaInvalid(data []byte) bool {
	return compileJSONSchema(data) != nil
}

func jsonSchemaTitleMissing(data []byte) bool {
	_, err := platformjsonschema.RootTitle(data)
	return err != nil
}

func compileJSONSchema(data []byte) error {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("jsonschema.UnmarshalJSON: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", document); err != nil {
		return fmt.Errorf("compiler.AddResource: %w", err)
	}
	_, err = compiler.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("compiler.Compile: %w", err)
	}
	return nil
}

func (s *Service) lintKafkaProto(ctx context.Context, project projectdomain.Project, result lintdomain.Result, target projectdomain.Target) (lintdomain.Result, error) {
	if s.proto == nil {
		return result, nil
	}
	if err := s.proto.Lint(ctx, project, target); err != nil {
		operationErr := &lintdomain.OperationError{Operation: lintdomain.OperationReadContract, Target: target.ID, Path: target.Input, Kind: lintdomain.FailureUnavailable, Cause: err}
		return result, fmt.Errorf("proto.Lint: %w", operationErr)
	}
	return result, nil
}

func (s *Service) lintGRPCTarget(ctx context.Context, project projectdomain.Project, result lintdomain.Result, target projectdomain.Target) (lintdomain.Result, error) {
	result.Contracts = append(result.Contracts, target.ID)
	if target.Role == "server" || target.Location.Local {
		files, err := s.contracts.ListProtoFiles(ctx, project.Root, target.Input)
		if err != nil {
			operationErr := &lintdomain.OperationError{Operation: lintdomain.OperationReadContract, Target: target.ID, Path: target.Input, Kind: lintdomain.FailureUnavailable, Cause: err}
			return result, fmt.Errorf("contracts.ListProtoFiles: %w", operationErr)
		}
		for _, filename := range files {
			if !protoFilenamePattern.MatchString(path.Base(filename)) {
				result.Issues = append(result.Issues, lintdomain.Issue{Code: "proto_filename", Target: target.ID, Path: filename})
			}
		}
	}
	result.Valid = len(result.Issues) == 0
	if s.proto != nil {
		if err := s.proto.Lint(ctx, project, target); err != nil {
			operationErr := &lintdomain.OperationError{Operation: lintdomain.OperationReadContract, Target: target.ID, Path: target.Input, Kind: lintdomain.FailureUnavailable, Cause: err}
			return result, fmt.Errorf("proto.Lint: %w", operationErr)
		}
	}
	return result, nil
}

func lintIssues(target projectdomain.Target, contractPath string, report platformopenapi.Report) []lintdomain.Issue {
	issues := make([]lintdomain.Issue, 0, len(report.Findings)+len(report.Operations))
	for _, finding := range report.Findings {
		issues = append(issues, lintdomain.Issue{
			Code: string(finding.Kind), Target: target.ID, Path: contractPath,
			Line: finding.Line, Column: finding.Column, Field: finding.Field,
			Parameters: &lintdomain.Parameters{
				Type: finding.Type, Subtype: finding.Subtype, SpecPath: finding.SpecPath,
			},
		})
	}
	if !strings.HasPrefix(report.Version, "3.1.") {
		issues = append(issues, lintdomain.Issue{Code: "openapi_version", Target: target.ID, Path: contractPath})
	}
	seen := map[string]struct{}{}
	for _, operation := range report.Operations {
		parameters := &lintdomain.Parameters{OperationID: operation.OperationID, Location: operation.Method + " " + operation.Path}
		switch {
		case operation.OperationID == "":
			issues = append(issues, lintdomain.Issue{Code: "operation_id_missing", Target: target.ID, Path: contractPath, Line: operation.Line, Column: operation.Column, Parameters: parameters})
		case hasOperationID(seen, operation.OperationID):
			issues = append(issues, lintdomain.Issue{Code: "operation_id_duplicate", Target: target.ID, Path: contractPath, Line: operation.Line, Column: operation.Column, Parameters: parameters})
		}
		if !hasSuccessfulResponse(operation.Responses) {
			issues = append(issues, lintdomain.Issue{Code: "response_2xx_missing", Target: target.ID, Path: contractPath, Line: operation.Line, Column: operation.Column, Parameters: parameters})
		}
	}
	return issues
}

func hasOperationID(seen map[string]struct{}, operationID string) bool {
	_, exists := seen[operationID]
	seen[operationID] = struct{}{}
	return exists
}

func hasSuccessfulResponse(responses []string) bool {
	for _, response := range responses {
		if response == "2XX" {
			return true
		}
		if len(response) == 3 && response[0] == '2' && response[1] >= '0' && response[1] <= '9' && response[2] >= '0' && response[2] <= '9' {
			return true
		}
	}
	return false
}
