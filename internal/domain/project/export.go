package project

import (
	"path"
	"strings"
)

// ExportMatchesSurface reports whether exported is an exact alias of one effective Project surface.
func (m Manifest) ExportMatchesSurface(exported Export) bool {
	switch exported.Kind {
	case "openapi":
		target, exists := m.target("http-server")
		return exists && exported.Producer == "" && validExportPath(exported.Path, false) && exported.Path == target.Reference.Entrypoint
	case "grpc":
		target, exists := m.target("grpc-server")
		return exists && exported.Producer == "" && validExportPath(exported.Path, true) && exported.Path == target.Reference.ProtoRoot
	case "kafka":
		_, exists := m.target("kafka-producer:" + exported.Producer)
		return exists && exported.Producer != "" && exported.Path == ""
	default:
		return false
	}
}

func (m Manifest) target(id string) (Target, bool) {
	for _, target := range NewTargetCatalog(m).All() {
		if target.ID == id {
			return target, true
		}
	}
	return Target{}, false
}

func validExportPath(name string, allowRoot bool) bool {
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	if allowRoot && clean == "." && name != "" {
		return true
	}
	return name != "" && clean != "." && clean != ".." && !strings.HasPrefix(clean, "/") && !strings.HasPrefix(clean, "../")
}
