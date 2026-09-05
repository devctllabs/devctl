package materialize

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/failure"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	"github.com/devctllabs/devctl/internal/domain/project"
)

const (
	maxURLDocuments    = 64
	maxURLResponseSize = 32 << 20
	maxURLSnapshotSize = 64 << 20
)

// URLService materializes a bounded relative-reference closure from a URL Source.
type URLService struct {
	client HTTPClient
}

func NewURL(client HTTPClient) *URLService { return &URLService{client: client} }

func (s *URLService) SourceType() project.SourceType { return project.SourceURL }

func (s *URLService) Materialize(ctx context.Context, request materializedomain.Request) (contract.Snapshot, error) {
	closure, err := newURLClosure(s.client, request)
	if err != nil {
		return contract.Snapshot{}, err
	}
	return closure.materialize(ctx)
}

type pendingURLDocument struct {
	fetchURL    string
	virtualPath string
}

type urlClosure struct {
	client        HTTPClient
	source        project.Source
	entrypoint    string
	virtualRoot   string
	queue         []pendingURLDocument
	seen          map[string]struct{}
	virtualOwners map[string]string
	files         []contract.File
	aggregateSize int
}

func newURLClosure(client HTTPClient, request materializedomain.Request) (*urlClosure, error) {
	if err := validateSourceURL(request.Source); err != nil {
		return nil, err
	}
	entrypoint := request.Reference.Entrypoint
	if entrypoint == "" {
		entrypoint = request.Source.Filename
	}
	if entrypoint == "" {
		entrypoint = "openapi.yaml"
	}
	if !safeRelative(entrypoint) {
		return nil, &materializedomain.OperationError{Operation: materializedomain.OperationValidateSource, SourceType: project.SourceURL, Path: entrypoint, Kind: materializedomain.FailureInvalid}
	}
	entrypoint = path.Clean(entrypoint)
	initial := pendingURLDocument{fetchURL: request.Source.URL, virtualPath: entrypoint}
	return &urlClosure{
		client: client, source: request.Source, entrypoint: entrypoint,
		virtualRoot: path.Dir(entrypoint),
		queue:       []pendingURLDocument{initial},
		seen:        make(map[string]struct{}),
		virtualOwners: map[string]string{
			entrypoint: fetchIdentity(initial.fetchURL),
		},
		files: make([]contract.File, 0, 1),
	}, nil
}

func (c *urlClosure) materialize(ctx context.Context) (contract.Snapshot, error) {
	for len(c.queue) > 0 {
		current := c.pop()
		identity := fetchIdentity(current.fetchURL)
		if _, exists := c.seen[identity]; exists {
			continue
		}
		if err := c.fetch(ctx, current, identity); err != nil {
			return contract.Snapshot{}, err
		}
	}
	return newSnapshot(c.entrypoint, c.files)
}

func (c *urlClosure) pop() pendingURLDocument {
	current := c.queue[0]
	c.queue = c.queue[1:]
	return current
}

func (c *urlClosure) fetch(ctx context.Context, current pendingURLDocument, identity string) error {
	if len(c.seen) >= maxURLDocuments {
		return &materializedomain.OperationError{Operation: materializedomain.OperationBuildSnapshot, SourceType: project.SourceURL, Path: current.virtualPath, Kind: materializedomain.FailureInvalid}
	}
	document, err := c.client.Fetch(ctx, materializedomain.HTTPFetchRequest{
		URL: current.fetchURL, OriginURL: c.source.URL,
		AllowInsecureHTTP: c.source.AllowInsecureHTTP,
	})
	if err != nil {
		operationErr := &materializedomain.OperationError{Operation: materializedomain.OperationDownload, SourceType: project.SourceURL, Path: redactedURL(current.fetchURL), Kind: downloadFailureKind(err), Cause: err}
		return fmt.Errorf("client.Fetch: %w", operationErr)
	}
	if err := c.add(current, document); err != nil {
		return err
	}
	c.seen[identity] = struct{}{}
	return c.enqueueReferences(current, document)
}

func (c *urlClosure) add(current pendingURLDocument, document materializedomain.HTTPDocument) error {
	if len(document.Content) > maxURLResponseSize || c.aggregateSize+len(document.Content) > maxURLSnapshotSize {
		return &materializedomain.OperationError{Operation: materializedomain.OperationBuildSnapshot, SourceType: project.SourceURL, Path: current.virtualPath, Kind: materializedomain.FailureInvalid}
	}
	c.aggregateSize += len(document.Content)
	c.files = append(c.files, contract.File{Path: current.virtualPath, Content: document.Content, Mode: 0o644})
	return nil
}

func (c *urlClosure) enqueueReferences(current pendingURLDocument, document materializedomain.HTTPDocument) error {
	base, err := url.Parse(document.URL)
	if err != nil {
		return &materializedomain.OperationError{Operation: materializedomain.OperationDownload, SourceType: project.SourceURL, Path: redactedURL(current.fetchURL), Kind: materializedomain.FailureUnavailable, Cause: err}
	}
	for _, rawReference := range collectReferences(document.Content) {
		resolved, include, resolveErr := c.resolveReference(current, base, rawReference)
		if resolveErr != nil {
			return resolveErr
		}
		if include {
			if enqueueErr := c.enqueue(resolved); enqueueErr != nil {
				return enqueueErr
			}
		}
	}
	return nil
}

func (c *urlClosure) enqueue(document pendingURLDocument) error {
	identity := fetchIdentity(document.fetchURL)
	if owner, exists := c.virtualOwners[document.virtualPath]; exists && owner != identity {
		return &materializedomain.OperationError{Operation: materializedomain.OperationBuildSnapshot, SourceType: project.SourceURL, Path: document.virtualPath, Kind: materializedomain.FailureInvalid}
	}
	c.virtualOwners[document.virtualPath] = identity
	c.queue = append(c.queue, document)
	return nil
}

func (c *urlClosure) resolveReference(current pendingURLDocument, base *url.URL, rawReference string) (pendingURLDocument, bool, error) {
	reference, err := url.Parse(rawReference)
	if err != nil {
		return pendingURLDocument{}, false, &materializedomain.OperationError{Operation: materializedomain.OperationValidateSource, SourceType: project.SourceURL, Path: redactedURL(rawReference), Kind: materializedomain.FailureInvalid, Cause: err}
	}
	if reference.IsAbs() || reference.Host != "" || strings.HasPrefix(reference.Path, "/") {
		return pendingURLDocument{}, false, nil
	}
	reference.Fragment = ""
	if reference.Path == "" && reference.RawQuery == "" {
		return pendingURLDocument{}, false, nil
	}
	virtualPath := current.virtualPath
	if reference.Path != "" {
		virtualPath = path.Clean(path.Join(path.Dir(current.virtualPath), reference.Path))
	}
	if !withinVirtualRoot(c.virtualRoot, virtualPath) {
		return pendingURLDocument{}, false, &materializedomain.OperationError{Operation: materializedomain.OperationValidateSource, SourceType: project.SourceURL, Path: redactedURL(rawReference), Kind: materializedomain.FailureInvalid}
	}
	return pendingURLDocument{fetchURL: base.ResolveReference(reference).String(), virtualPath: virtualPath}, true, nil
}

func downloadFailureKind(err error) materializedomain.FailureKind {
	switch failure.CategoryOf(err) {
	case failure.InvalidInput:
		return materializedomain.FailureInvalid
	case failure.NotFound:
		return materializedomain.FailureNotFound
	case failure.Unsupported:
		return materializedomain.FailureUnsupported
	case failure.Conflict, failure.Unavailable, failure.Cancelled, failure.Internal:
		return materializedomain.FailureUnavailable
	}
	return materializedomain.FailureUnavailable
}

func redactedURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid-url>"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

func withinVirtualRoot(root, name string) bool {
	if !safeRelative(name) {
		return false
	}
	return root == "." || strings.HasPrefix(name, root+"/")
}

func fetchIdentity(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.Fragment = ""
	return parsed.String()
}

func validateSourceURL(source project.Source) error {
	parsed, err := url.Parse(source.URL)
	if err != nil || parsed.Host == "" {
		return &materializedomain.OperationError{Operation: materializedomain.OperationValidateSource, SourceType: project.SourceURL, Path: redactedURL(source.URL), Kind: materializedomain.FailureInvalid}
	}
	if parsed.User != nil {
		return &materializedomain.OperationError{Operation: materializedomain.OperationValidateSource, SourceType: project.SourceURL, Path: redactedURL(source.URL), Kind: materializedomain.FailureInvalid}
	}
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !source.AllowInsecureHTTP) {
		return &materializedomain.OperationError{Operation: materializedomain.OperationValidateSource, SourceType: project.SourceURL, Path: redactedURL(source.URL), Kind: materializedomain.FailureInvalid}
	}
	return nil
}
