package jsonschema_test

import (
	"testing"

	platformjsonschema "github.com/devctllabs/devctl/internal/platform/jsonschema"
	"github.com/stretchr/testify/require"
)

func TestRootTitle(t *testing.T) {
	t.Parallel()

	title, err := platformjsonschema.RootTitle([]byte(`{"title":" AuditEvent ","type":"object"}`))

	require.NoError(t, err)
	require.Equal(t, "AuditEvent", title)
}

func TestRootTitleRejectsMissingOrInvalidTitle(t *testing.T) {
	t.Parallel()

	for _, schema := range []string{`{"type":"object"}`, `{"title":" "}`, `{`} {
		_, err := platformjsonschema.RootTitle([]byte(schema))

		require.Error(t, err)
	}
}
