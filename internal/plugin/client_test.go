package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_ListBlacklist(t *testing.T) {
	testCases := []struct {
		desc       string
		baseURL    string
		statusCode int
		body       string
		expected   []BlacklistEntry
		expectErr  require.ErrorAssertionFunc
		apiErr     *APIError
	}{
		{
			desc:       "entries",
			baseURL:    "/internal/",
			statusCode: http.StatusOK,
			body:       `[{"repository":"deas/teectl","reason":"Not a plugin","author":"ldez","createdAt":"2026-01-02T03:04:05Z"}]`,
			expected: []BlacklistEntry{{
				Repository: "deas/teectl",
				Reason:     "Not a plugin",
				Author:     "ldez",
				CreatedAt:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			}},
			expectErr: require.NoError,
		},
		{
			desc:       "base URL without trailing slash",
			baseURL:    "/internal",
			statusCode: http.StatusOK,
			body:       `[]`,
			expected:   []BlacklistEntry{},
			expectErr:  require.NoError,
		},
		{
			desc:       "API error",
			baseURL:    "/internal/",
			statusCode: http.StatusInternalServerError,
			body:       "boom",
			expectErr:  require.Error,
			apiErr:     &APIError{StatusCode: http.StatusInternalServerError, Message: "boom"},
		},
		{
			desc:       "invalid JSON",
			baseURL:    "/internal/",
			statusCode: http.StatusOK,
			body:       `{`,
			expectErr:  require.Error,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()
			mux.HandleFunc("GET /internal/blacklist", func(rw http.ResponseWriter, _ *http.Request) {
				rw.WriteHeader(test.statusCode)
				_, _ = rw.Write([]byte(test.body))
			})

			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)

			client := New(server.URL + test.baseURL)

			entries, err := client.ListBlacklist(context.Background())
			test.expectErr(t, err)

			if test.apiErr != nil {
				var apiErr *APIError
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, test.apiErr, apiErr)
			}

			assert.Equal(t, test.expected, entries)
		})
	}
}
