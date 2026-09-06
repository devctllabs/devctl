package manifest

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"gopkg.in/yaml.v3"
)

func parse(data []byte) (document, []projectdomain.DecodeIssue, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return document{}, []projectdomain.DecodeIssue{{Kind: projectdomain.DecodeYAMLInvalid}}, fmt.Errorf("yaml.Unmarshal: %w", err)
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		err := errors.New("manifest root must be a mapping")
		return document{}, []projectdomain.DecodeIssue{{Kind: projectdomain.DecodeSchemaInvalid}}, err
	}
	issues := validateKnownFields(node.Content[0], "")
	if len(issues) > 0 {
		return document{}, issues, errors.New("manifest schema validation failed")
	}
	var manifest document
	if err := node.Content[0].Decode(&manifest); err != nil {
		return document{}, []projectdomain.DecodeIssue{{Kind: projectdomain.DecodeSchemaInvalid}}, fmt.Errorf("node.Decode: %w", err)
	}
	return manifest, issues, nil
}

func validateKnownFields(root *yaml.Node, path string) []projectdomain.DecodeIssue {
	issues := make([]projectdomain.DecodeIssue, 0)
	validateNode(root, reflect.TypeFor[document](), path, &issues)
	return issues
}

func validateNode(node *yaml.Node, target reflect.Type, path string, issues *[]projectdomain.DecodeIssue) {
	if node.Tag == "!!null" {
		return
	}
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	kind := target.Kind()
	if kind == reflect.Struct {
		validateStructNode(node, target, path, issues)
		return
	}
	if kind == reflect.Map {
		validateMapNode(node, target.Elem(), path, issues)
		return
	}
	if kind == reflect.Slice {
		validateSliceNode(node, target.Elem(), path, issues)
		return
	}
	if kind == reflect.Interface {
		return
	}
	validateScalarNode(node, target, path, issues)
}

func validateStructNode(node *yaml.Node, target reflect.Type, path string, issues *[]projectdomain.DecodeIssue) {
	if node.Kind != yaml.MappingNode {
		appendSchemaIssue(issues, path, node)
		return
	}
	fields := yamlFields(target)
	visitMapping(node, path, issues, func(value *yaml.Node, fieldPath, name string) {
		fieldType, exists := fields[name]
		if !exists {
			key := mappingKey(node, name)
			*issues = append(*issues, projectdomain.DecodeIssue{Kind: projectdomain.DecodeUnknownField, Field: fieldPath, Line: key.Line, Column: key.Column})
			return
		}
		validateNode(value, fieldType, fieldPath, issues)
	})
}

func validateMapNode(node *yaml.Node, element reflect.Type, path string, issues *[]projectdomain.DecodeIssue) {
	if node.Kind != yaml.MappingNode {
		appendSchemaIssue(issues, path, node)
		return
	}
	visitMapping(node, path, issues, func(value *yaml.Node, fieldPath, _ string) {
		validateNode(value, element, fieldPath, issues)
	})
}

func validateSliceNode(node *yaml.Node, element reflect.Type, path string, issues *[]projectdomain.DecodeIssue) {
	if node.Kind != yaml.SequenceNode {
		appendSchemaIssue(issues, path, node)
		return
	}
	for index, item := range node.Content {
		validateNode(item, element, fmt.Sprintf("%s[%d]", path, index), issues)
	}
}

func validateScalarNode(node *yaml.Node, target reflect.Type, path string, issues *[]projectdomain.DecodeIssue) {
	if node.Kind != yaml.ScalarNode {
		appendSchemaIssue(issues, path, node)
		return
	}
	value := reflect.New(target).Interface()
	if err := node.Decode(value); err != nil {
		appendSchemaIssue(issues, path, node)
	}
}

func visitMapping(
	node *yaml.Node,
	path string,
	issues *[]projectdomain.DecodeIssue,
	visit func(value *yaml.Node, fieldPath, name string),
) {
	seen := make(map[string]struct{}, len(node.Content)/2)
	for index := 0; index+1 < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		fieldPath := joinFieldPath(path, key.Value)
		if _, exists := seen[key.Value]; exists {
			*issues = append(*issues, projectdomain.DecodeIssue{Kind: projectdomain.DecodeDuplicateKey, Field: fieldPath, Line: key.Line, Column: key.Column})
			continue
		}
		seen[key.Value] = struct{}{}
		visit(value, fieldPath, key.Value)
	}
}

func yamlFields(target reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, target.NumField())
	for index := 0; index < target.NumField(); index++ {
		field := target.Field(index)
		name := strings.SplitN(field.Tag.Get("yaml"), ",", 2)[0]
		if name != "" && name != "-" {
			fields[name] = field.Type
		}
	}
	return fields
}

func mappingKey(node *yaml.Node, name string) *yaml.Node {
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == name {
			return node.Content[index]
		}
	}
	return node
}

func appendSchemaIssue(issues *[]projectdomain.DecodeIssue, path string, node *yaml.Node) {
	*issues = append(*issues, projectdomain.DecodeIssue{Kind: projectdomain.DecodeSchemaInvalid, Field: path, Line: node.Line, Column: node.Column})
}

func joinFieldPath(parent, field string) string {
	if parent == "" {
		return field
	}
	return parent + "." + field
}
