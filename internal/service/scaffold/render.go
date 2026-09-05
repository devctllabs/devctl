package scaffold

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
	"unicode"
)

//go:embed templates/*
var scaffoldTemplates embed.FS

func readTemplateAsset(name string) ([]byte, error) {
	content, err := scaffoldTemplates.ReadFile("templates/" + name)
	if err != nil {
		return nil, fmt.Errorf("scaffoldTemplates.ReadFile: %w", err)
	}
	return content, nil
}

func executeTemplate(name string, data any) (string, error) {
	parsed, err := template.New(name).Funcs(template.FuncMap{
		"goName":  goName,
		"replace": func(value string) string { return strings.ReplaceAll(value, "-", "_") },
		"hasProto": func(consumers []kafkaConsumerTemplateData) bool {
			for _, consumer := range consumers {
				if consumer.Format == "proto" {
					return true
				}
			}
			return false
		},
	}).ParseFS(scaffoldTemplates, "templates/"+name)
	if err != nil {
		return "", fmt.Errorf("template.ParseFS: %w", err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return "", fmt.Errorf("parsed.Execute: %w", err)
	}
	return output.String(), nil
}

type goModTemplateData struct {
	Module   string
	Requires []string
	HTTP     bool
	Proto    bool
}

func renderGoMod(data goModTemplateData) (string, error) {
	return executeTemplate("go.mod.gotmpl", data)
}

type migrationTask struct {
	TaskPrefix         string
	Path               string
	DatabaseExpression string
	Kind               string
}

type miseTemplateData struct {
	JSON          bool
	Migrations    []migrationTask
	MigrationTags []string
}

func renderMise(data miseTemplateData) (string, error) {
	rendered, err := executeTemplate("mise.toml", data)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(rendered, "\n") + "\n", nil
}

type mainTemplateData struct {
	Project string
	Module  string
	Server  bool
	Kafka   bool
}

func renderMain(data mainTemplateData) (string, error) {
	return executeTemplate("main.go.gotmpl", data)
}

func renderAPI(module string) (string, error) {
	return executeTemplate("api.go.gotmpl", struct{ Module string }{Module: module})
}

func unique(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func goName(value string) string {
	var builder strings.Builder
	upper := true
	for _, char := range value {
		if char == '-' || char == '_' {
			upper = true
			continue
		}
		if upper {
			char = unicode.ToUpper(char)
			upper = false
		}
		builder.WriteRune(char)
	}
	return builder.String()
}
