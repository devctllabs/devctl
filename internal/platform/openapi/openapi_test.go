package openapi_test

import (
	"testing"

	"github.com/devctllabs/devctl/internal/platform/openapi"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeReturnsMalformedDocumentFact(t *testing.T) {
	t.Parallel()

	report := openapi.Analyze([]byte("openapi: [\n"))

	require.Equal(t, []openapi.Finding{{Kind: openapi.DocumentInvalid}}, report.Findings)
}

func TestAnalyzeExtractsOperationFacts(t *testing.T) {
	t.Parallel()

	report := openapi.Analyze([]byte(`openapi: 3.1.0
info: {title: Fixture, version: 1.0.0}
paths:
  /widgets:
    get:
      operationId: listWidgets
      responses:
        2XX: {description: success}
`))

	require.Equal(t, "3.1.0", report.Version)
	require.Equal(t, []openapi.Operation{{
		Method: "GET", Path: "/widgets", OperationID: "listWidgets", Responses: []string{"2XX"}, Line: 5, Column: 5,
	}}, report.Operations)
}
