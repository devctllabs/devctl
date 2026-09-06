package materialize

import (
	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/project"
)

// Request selects a contract closure from one source within a project root.
type Request struct {
	// Root is the containment boundary for project-relative source paths.
	Root      string
	Source    project.Source
	Reference contract.Reference
}

// HTTPFetchRequest describes one bounded fetch within a URL Source origin.
type HTTPFetchRequest struct {
	URL               string
	OriginURL         string
	AllowInsecureHTTP bool
}

// HTTPDocument is one fetched Contract document and its effective URL.
type HTTPDocument struct {
	URL     string
	Content []byte
}
