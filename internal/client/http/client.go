package http

import (
	"context"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"

	"github.com/devctllabs/devctl/internal/domain/failure"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
)

const maxResponseSize = 32 << 20

// Client retrieves bounded contract documents with a fixed request timeout.
type Client struct {
	client *stdhttp.Client
}

func New() *Client {
	return &Client{client: &stdhttp.Client{Timeout: 30 * time.Second}}
}

// Fetch accepts only 2xx responses, limits bodies to 32 MiB, and returns the effective response URL.
func (c *Client) Fetch(ctx context.Context, fetch materializedomain.HTTPFetchRequest) (materializedomain.HTTPDocument, error) {
	request, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, fetch.URL, nil)
	if err != nil {
		return materializedomain.HTTPDocument{}, fmt.Errorf("stdhttp.NewRequestWithContext: %w", err)
	}
	origin, err := url.Parse(fetch.OriginURL)
	if err != nil || origin.Host == "" {
		return materializedomain.HTTPDocument{}, &PolicyError{Reason: "invalid origin URL", Cause: err}
	}
	if err := validateURLPolicy(origin, request.URL, fetch.AllowInsecureHTTP); err != nil {
		return materializedomain.HTTPDocument{}, err
	}
	client := *c.client
	client.CheckRedirect = redirectPolicy(origin, fetch.AllowInsecureHTTP)
	response, err := client.Do(request)
	if err != nil {
		category := failure.CategoryOf(err)
		if category == failure.Internal {
			category = failure.Unavailable
		}
		return materializedomain.HTTPDocument{}, &FetchError{Kind: category, Cause: err}
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return materializedomain.HTTPDocument{}, &StatusError{StatusCode: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return materializedomain.HTTPDocument{}, fmt.Errorf("io.ReadAll: %w", err)
	}
	if len(data) > maxResponseSize {
		return materializedomain.HTTPDocument{}, &BodyTooLargeError{Limit: maxResponseSize}
	}
	return materializedomain.HTTPDocument{URL: response.Request.URL.String(), Content: data}, nil
}

// redirectPolicy rejects origin changes, credentials, disallowed schemes, and excessive redirect chains.
func redirectPolicy(origin *url.URL, allowInsecureHTTP bool) func(*stdhttp.Request, []*stdhttp.Request) error {
	return func(request *stdhttp.Request, via []*stdhttp.Request) error {
		if len(via) >= 10 {
			return &PolicyError{Reason: "too many redirects"}
		}
		return validateURLPolicy(origin, request.URL, allowInsecureHTTP)
	}
}

func validateURLPolicy(origin, target *url.URL, allowInsecureHTTP bool) error {
	if origin.User != nil || target.User != nil {
		return &PolicyError{Reason: "URL contains credentials"}
	}
	targetScheme := strings.ToLower(target.Scheme)
	if targetScheme != "https" && (targetScheme != "http" || !allowInsecureHTTP) {
		return &PolicyError{Reason: "URL uses a disallowed scheme"}
	}
	if !sameOrigin(origin, target) {
		return &PolicyError{Reason: "URL changes source origin"}
	}
	return nil
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if value.Port() != "" {
		return value.Port()
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

// PolicyError reports a URL request that violates Source fetch policy.
type PolicyError struct {
	Reason string
	Cause  error
}

func (e *PolicyError) Error() string { return e.Reason }
func (e *PolicyError) Unwrap() error { return e.Cause }
func (e *PolicyError) Category() failure.Category {
	return failure.InvalidInput
}

// FetchError retains the raw transport cause behind a query-safe public message.
type FetchError struct {
	Kind  failure.Category
	Cause error
}

func (e *FetchError) Error() string              { return "HTTP fetch failed" }
func (e *FetchError) Unwrap() error              { return e.Cause }
func (e *FetchError) Category() failure.Category { return e.Kind }

// StatusError retains a non-success HTTP response status without interpreting application policy.
type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string { return fmt.Sprintf("HTTP status %d", e.StatusCode) }

func (e *StatusError) Category() failure.Category {
	if e.StatusCode == stdhttp.StatusNotFound || e.StatusCode == stdhttp.StatusGone {
		return failure.NotFound
	}
	return failure.Unavailable
}

// BodyTooLargeError reports the byte limit exceeded by a response body.
type BodyTooLargeError struct {
	Limit int
}

func (e *BodyTooLargeError) Error() string { return fmt.Sprintf("response exceeds %d bytes", e.Limit) }

func (e *BodyTooLargeError) Category() failure.Category { return failure.InvalidInput }
