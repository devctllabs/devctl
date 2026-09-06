package project

import projectdomain "github.com/devctllabs/devctl/internal/domain/project"

func effectiveEnv(catalog projectdomain.RuntimeConfigCatalog) []projectdomain.EffectiveEnv {
	fields := catalog.Entries(projectdomain.RuntimeConfigInspect)
	result := make([]projectdomain.EffectiveEnv, len(fields))
	for index, field := range fields {
		var defaultValue any
		if field.HasDefault && !field.Secret {
			defaultValue = field.Default
		}
		result[index] = projectdomain.EffectiveEnv{
			Key: field.Key, Type: string(field.Type), Default: defaultValue, Secret: field.Secret,
		}
	}
	return result
}
