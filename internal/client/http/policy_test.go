package http

import (
	"net/url"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/failure"
	"github.com/stretchr/testify/require"
)

func TestSameOriginUsesSchemeHostAndEffectivePort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		left     string
		right    string
		expected bool
	}{
		{name: "https default port", left: "https://example.test/a", right: "https://EXAMPLE.test:443/b", expected: true},
		{name: "http default port", left: "http://example.test/a", right: "http://example.test:80/b", expected: true},
		{name: "different explicit port", left: "https://example.test/a", right: "https://example.test:8443/b", expected: false},
		{name: "different scheme", left: "http://example.test/a", right: "https://example.test/b", expected: false},
		{name: "different host", left: "https://example.test/a", right: "https://other.test/b", expected: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			left, err := url.Parse(test.left)
			require.NoError(t, err)
			right, err := url.Parse(test.right)
			require.NoError(t, err)

			require.Equal(t, test.expected, sameOrigin(left, right))
		})
	}
}

func TestURLPolicyRejectsCredentialsAndInsecureScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		origin        string
		target        string
		allowInsecure bool
	}{
		{name: "origin credentials", origin: "https://user:secret@example.test/a", target: "https://example.test/b"},
		{name: "target credentials", origin: "https://example.test/a", target: "https://user:secret@example.test/b"},
		{name: "insecure scheme", origin: "http://example.test/a", target: "http://example.test/b"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			origin, err := url.Parse(test.origin)
			require.NoError(t, err)
			target, err := url.Parse(test.target)
			require.NoError(t, err)

			err = validateURLPolicy(origin, target, test.allowInsecure)

			require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
			require.NotContains(t, err.Error(), "secret")
		})
	}
}
