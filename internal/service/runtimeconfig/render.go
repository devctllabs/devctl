package runtimeconfig

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

//go:embed templates/config.go.gotmpl
var configTemplate string

// Output contains the config package and project-root files owned by Runtime Config.
type Output struct {
	Directory artifact.Tree
	Files     artifact.Tree
}

// Render converts a canonical catalog into deterministic Managed Output.
func Render(catalog projectdomain.RuntimeConfigCatalog) (Output, error) {
	configFile, err := renderConfig(catalog.Entries(projectdomain.RuntimeConfigRuntime))
	if err != nil {
		return Output{}, err
	}
	envFile := renderEnv(catalog.Entries(projectdomain.RuntimeConfigExample))
	return Output{
		Directory: artifact.Tree{Files: []artifact.File{{Path: "config.gen.go", Content: configFile, Mode: 0o644}}},
		Files:     artifact.Tree{Files: []artifact.File{{Path: ".env.example", Content: envFile, Mode: 0o644}}},
	}, nil
}

type templateGroup struct {
	Name   string
	Fields []templateField
}

type templateField struct {
	Name string
	Type string
	Tag  string
}

func renderConfig(fields []projectdomain.RuntimeConfigField) ([]byte, error) {
	grouped := make(map[string][]projectdomain.RuntimeConfigField)
	for _, field := range fields {
		grouped[field.Group] = append(grouped[field.Group], field)
	}
	groupNames := make([]string, 0, len(grouped))
	for name := range grouped {
		groupNames = append(groupNames, name)
	}
	sort.Strings(groupNames)

	groups := make([]templateGroup, 0, len(groupNames))
	for _, groupName := range groupNames {
		groupFields := grouped[groupName]
		sort.Slice(groupFields, func(i, j int) bool { return groupFields[i].Name < groupFields[j].Name })
		group := templateGroup{Name: groupName, Fields: make([]templateField, 0, len(groupFields))}
		for _, field := range groupFields {
			group.Fields = append(group.Fields, templateField{Name: field.Name, Type: goType(field.Type), Tag: fieldTag(field)})
		}
		groups = append(groups, group)
	}

	parsed, err := template.New("config.go").Parse(configTemplate)
	if err != nil {
		return nil, fmt.Errorf("template.Parse: %w", err)
	}
	var rendered bytes.Buffer
	if err := parsed.Execute(&rendered, groups); err != nil {
		return nil, fmt.Errorf("parsed.Execute: %w", err)
	}
	formatted, err := format.Source(rendered.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format.Source: %w", err)
	}
	return formatted, nil
}

func fieldTag(field projectdomain.RuntimeConfigField) string {
	tag := "`env:" + strconv.Quote(field.Key)
	if field.HasDefault && !field.Secret {
		tag += " default:" + strconv.Quote(fmt.Sprint(field.Default))
	}
	return tag + "`"
}

func goType(value projectdomain.RuntimeConfigType) string {
	switch value {
	case projectdomain.RuntimeConfigBool:
		return "bool"
	case projectdomain.RuntimeConfigInt:
		return "int"
	case projectdomain.RuntimeConfigDuration:
		return "time.Duration"
	case projectdomain.RuntimeConfigStringList:
		return "[]string"
	case projectdomain.RuntimeConfigString:
		return "string"
	default:
		return "string"
	}
}

func renderEnv(fields []projectdomain.RuntimeConfigField) []byte {
	var builder strings.Builder
	for _, field := range fields {
		value := ""
		if field.HasDefault && !field.Secret {
			value = fmt.Sprint(field.Default)
		}
		fmt.Fprintf(&builder, "%s=%s\n", field.Key, value)
	}
	return []byte(builder.String())
}
