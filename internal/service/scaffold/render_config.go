package scaffold

import (
	"fmt"
	"io/fs"
	"path"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/runtimeconfig"
)

type configLoaderTemplateData struct {
	ImportPath string
}

type configProjection struct {
	catalog         projectdomain.RuntimeConfigCatalog
	catalogErr      error
	target          projectdomain.Target
	targetAvailable bool
	importPath      string
}

func runtimeConfigArtifacts(projection configProjection) ([]Artifact, error) {
	if projection.catalogErr != nil {
		return nil, fmt.Errorf("project.NewRuntimeConfigCatalog: %w", projection.catalogErr)
	}
	output, err := runtimeconfig.Render(projection.catalog)
	if err != nil {
		return nil, fmt.Errorf("runtimeconfig.Render: %w", err)
	}
	if !projection.targetAvailable {
		return nil, fmt.Errorf("effective config target is unavailable")
	}
	loader, err := executeTemplate("config.go.gotmpl", configLoaderTemplateData{
		ImportPath: projection.importPath,
	})
	if err != nil {
		return nil, fmt.Errorf("executeTemplate: %w", err)
	}
	artifacts := []Artifact{{Path: "internal/deps/config.gen.go", Mode: 0o644, Content: []byte(loader)}}
	for _, file := range output.Directory.Files {
		artifacts = append(artifacts, Artifact{Path: path.Join(projection.target.OutputDir, file.Path), Mode: fs.FileMode(file.Mode), Content: file.Content})
	}
	for _, file := range output.Files.Files {
		artifacts = append(artifacts, Artifact{Path: file.Path, Mode: fs.FileMode(file.Mode), Content: file.Content})
	}
	return artifacts, nil
}
