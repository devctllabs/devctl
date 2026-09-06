package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	httpclient "github.com/devctllabs/devctl/internal/client/http"
	"github.com/devctllabs/devctl/internal/domain/failure"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	"github.com/stretchr/testify/require"
)

func TestClientGetsResponseBody(t *testing.T) {
	t.Parallel()

	methods := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		methods <- request.Method
		_, _ = writer.Write([]byte("openapi: 3.1.0\n"))
	}))
	t.Cleanup(server.Close)

	document, err := httpclient.New().Fetch(context.Background(), materializedomain.HTTPFetchRequest{
		URL: server.URL, OriginURL: server.URL, AllowInsecureHTTP: true,
	})

	require.NoError(t, err)
	require.Equal(t, http.MethodGet, <-methods)
	require.Equal(t, server.URL, document.URL)
	require.Equal(t, []byte("openapi: 3.1.0\n"), document.Content)
}

func TestClientRejectsCrossOriginRedirect(t *testing.T) {
	t.Parallel()

	destination := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("should not be fetched"))
	}))
	t.Cleanup(destination.Close)
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Redirect(writer, &http.Request{}, destination.URL+"/contract.yaml", http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	_, err := httpclient.New().Fetch(context.Background(), materializedomain.HTTPFetchRequest{
		URL: origin.URL, OriginURL: origin.URL, AllowInsecureHTTP: true,
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
}

func TestClientReturnsEffectiveURLAfterSameOriginRedirect(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/start" {
			http.Redirect(writer, request, "/nested/contract.yaml", http.StatusFound)
			return
		}
		_, _ = writer.Write([]byte("openapi: 3.1.0\n"))
	}))
	t.Cleanup(server.Close)

	document, err := httpclient.New().Fetch(context.Background(), materializedomain.HTTPFetchRequest{
		URL: server.URL + "/start", OriginURL: server.URL + "/start", AllowInsecureHTTP: true,
	})

	require.NoError(t, err)
	require.Equal(t, server.URL+"/nested/contract.yaml", document.URL)
}

func TestClientClassifiesHTTPStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status   int
		expected failure.Category
	}{
		{status: http.StatusNotFound, expected: failure.NotFound},
		{status: http.StatusGone, expected: failure.NotFound},
		{status: http.StatusInternalServerError, expected: failure.Unavailable},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
			}))
			t.Cleanup(server.Close)

			_, err := httpclient.New().Fetch(context.Background(), materializedomain.HTTPFetchRequest{
				URL: server.URL, OriginURL: server.URL, AllowInsecureHTTP: true,
			})

			require.Equal(t, test.expected, failure.CategoryOf(err))
		})
	}
}

func TestClientClassifiesOversizedResponseAsInvalidInput(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(make([]byte, (32<<20)+1))
	}))
	t.Cleanup(server.Close)

	_, err := httpclient.New().Fetch(context.Background(), materializedomain.HTTPFetchRequest{
		URL: server.URL, OriginURL: server.URL, AllowInsecureHTTP: true,
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
	var sizeErr *httpclient.BodyTooLargeError
	require.ErrorAs(t, err, &sizeErr)
}

func TestClientClassifiesTransportFailureAsUnavailable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	serverURL := server.URL
	server.Close()

	_, err := httpclient.New().Fetch(context.Background(), materializedomain.HTTPFetchRequest{
		URL: serverURL, OriginURL: serverURL, AllowInsecureHTTP: true,
	})

	require.Equal(t, failure.Unavailable, failure.CategoryOf(err))
}

func TestClientReturnsTypedStatusError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	_, err := httpclient.New().Fetch(context.Background(), materializedomain.HTTPFetchRequest{
		URL: server.URL, OriginURL: server.URL, AllowInsecureHTTP: true,
	})

	var statusError *httpclient.StatusError
	require.ErrorAs(t, err, &statusError)
	require.Equal(t, http.StatusBadGateway, statusError.StatusCode)
}
