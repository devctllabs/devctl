package project_test

import (
	"testing"

	"github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestValidateReportsIdentityAndPathIssuesInStableOrder(t *testing.T) {
	t.Parallel()

	selected := project.Project{
		ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{
			Version: 2,
			Project: project.Identity{Name: "Invalid Name", Language: "python"},
			Paths:   project.ManifestPaths{ExternalContracts: "../external"},
			Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{
				Config: &project.ConfigGenerator{Out: "/generated/config"},
			}}},
		},
	}

	issues := project.Validate(selected)

	require.Equal(t, []project.Issue{
		{Code: project.IssueVersionUnsupported, Path: selected.ManifestPath, Field: "version"},
		{Code: project.IssueNameInvalid, Path: selected.ManifestPath, Field: "project.name"},
		{Code: project.IssueLanguageUnsupported, Path: selected.ManifestPath, Field: "project.language"},
		{Code: project.IssueGoModuleRequired, Path: selected.ManifestPath, Field: "languages.go.module"},
		{Code: project.IssuePathInvalid, Path: selected.ManifestPath, Field: "paths.external_contracts"},
		{Code: project.IssuePathInvalid, Path: selected.ManifestPath, Field: "paths.external_contracts"},
		{Code: project.IssuePathInvalid, Path: selected.ManifestPath, Field: "languages.go.generators.config.out"},
	}, issues)
}
