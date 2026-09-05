package openapi

import (
	"sort"
	"strings"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"gopkg.in/yaml.v3"
)

// FindingKind classifies protocol-level parse, reference, and standard-validation facts.
type FindingKind string

const (
	DocumentInvalid  FindingKind = "document_invalid"
	ReferenceInvalid FindingKind = "reference_invalid"
	StandardInvalid  FindingKind = "openapi_standard"
)

// Finding is one presentation-neutral OpenAPI analysis fact.
type Finding struct {
	Kind     FindingKind
	Type     string
	Subtype  string
	SpecPath string
	Field    string
	// Line and Column are one-based; zero means the location is unavailable.
	Line   int
	Column int
}

// Operation contains the fields needed by devctl rules for one OpenAPI operation.
type Operation struct {
	Method      string
	Path        string
	OperationID string
	Responses   []string
	// Line and Column are one-based source coordinates.
	Line   int
	Column int
}

// Report contains deterministic facts extracted without performing I/O.
type Report struct {
	Version    string
	Operations []Operation
	Findings   []Finding
}

// Analyze parses and validates one OpenAPI document without applying devctl-specific lint policy.
func Analyze(data []byte) Report {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return Report{Operations: []Operation{}, Findings: []Finding{{Kind: DocumentInvalid}}}
	}
	root := documentRoot(&document)
	report := Report{
		Version: mappingScalar(root, "openapi"), Operations: operations(root), Findings: []Finding{},
	}
	openAPIDocument, err := libopenapi.NewDocument(data)
	if err != nil {
		report.Findings = append(report.Findings, Finding{Kind: DocumentInvalid})
		return report
	}
	defer openAPIDocument.Release()
	if _, err := openAPIDocument.BuildV3Model(); err != nil {
		report.Findings = append(report.Findings, Finding{Kind: ReferenceInvalid})
		return report
	}
	openAPIValidator, setupErrors := validator.NewValidator(openAPIDocument)
	if len(setupErrors) > 0 {
		report.Findings = append(report.Findings, Finding{Kind: StandardInvalid})
		return report
	}
	defer openAPIValidator.Release()
	valid, validationErrors := openAPIValidator.ValidateDocument()
	if !valid {
		for range validationErrors {
			report.Findings = append(report.Findings, Finding{Kind: StandardInvalid})
		}
	}
	return report
}

func operations(root *yaml.Node) []Operation {
	paths := mappingValue(root, "paths")
	if paths == nil || paths.Kind != yaml.MappingNode {
		return []Operation{}
	}
	methods := map[string]bool{"get": true, "put": true, "post": true, "delete": true, "options": true, "head": true, "patch": true, "trace": true}
	result := []Operation{}
	for index := 0; index+1 < len(paths.Content); index += 2 {
		pathName, item := paths.Content[index], paths.Content[index+1]
		if item.Kind != yaml.MappingNode {
			continue
		}
		for operationIndex := 0; operationIndex+1 < len(item.Content); operationIndex += 2 {
			methodNode, operationNode := item.Content[operationIndex], item.Content[operationIndex+1]
			method := strings.ToLower(methodNode.Value)
			if !methods[method] || operationNode.Kind != yaml.MappingNode {
				continue
			}
			responses := mappingKeys(mappingValue(operationNode, "responses"))
			result = append(result, Operation{
				Method: strings.ToUpper(method), Path: pathName.Value,
				OperationID: mappingScalar(operationNode, "operationId"), Responses: responses,
				Line: methodNode.Line, Column: methodNode.Column,
			})
		}
	}
	return result
}

func mappingKeys(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.MappingNode {
		return []string{}
	}
	keys := make([]string, 0, len(node.Content)/2)
	for index := 0; index+1 < len(node.Content); index += 2 {
		keys = append(keys, node.Content[index].Value)
	}
	sort.Strings(keys)
	return keys
}

func documentRoot(node *yaml.Node) *yaml.Node {
	if node != nil && node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return node.Content[0]
	}
	return node
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func mappingScalar(node *yaml.Node, key string) string {
	value := mappingValue(node, key)
	if value == nil {
		return ""
	}
	return value.Value
}
