package project

import "sort"

// MigrationTarget is the effective migration configuration of one database Variant.
type MigrationTarget struct {
	Connection      string
	Variant         string
	Kind            string
	Path            string
	DatabaseEnv     string
	DatabaseDefault string
}

// ID returns the CLI selector of this Migration Target.
func (t MigrationTarget) ID() string { return t.Connection + ":" + t.Variant }

// MigrationTargets returns migration-enabled Variants in stable selector order.
func MigrationTargets(manifest Manifest) []MigrationTarget {
	var targets []MigrationTarget
	if manifest.Components.DB == nil {
		return targets
	}
	prefix := runtimeConfigPrefix(manifest)
	for _, connection := range manifest.Components.DB.Connections {
		for _, variant := range connection.Variants {
			if variant.Migrations == nil {
				continue
			}
			migrations := variant.Migrations
			targets = append(targets, MigrationTarget{
				Connection: connection.Name, Variant: variant.Name, Kind: variant.Kind,
				Path: migrations.Path, DatabaseEnv: prefix + migrations.DatabaseEnv, DatabaseDefault: migrations.DatabaseDefault,
			})
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID() < targets[j].ID() })
	return targets
}
