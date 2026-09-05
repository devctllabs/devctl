package generate

import (
	"fmt"

	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/runtimeconfig"
)

func generateConfig(manifest projectdomain.Manifest) (generatedomain.Output, error) {
	catalog, err := projectdomain.NewRuntimeConfigCatalog(manifest)
	if err != nil {
		return generatedomain.Output{}, fmt.Errorf("project.NewRuntimeConfigCatalog: %w", err)
	}
	output, err := runtimeconfig.Render(catalog)
	if err != nil {
		return generatedomain.Output{}, fmt.Errorf("runtimeconfig.Render: %w", err)
	}
	return generatedomain.Output{Directory: output.Directory, Files: output.Files}, nil
}
