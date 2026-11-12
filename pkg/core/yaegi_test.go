package core

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrapper_checkProxyVersion(t *testing.T) {
	testCases := []struct {
		desc          string
		status        int
		expectErr     string
		notPluginProb bool
	}{
		{
			desc:   "the proxy serves the version",
			status: http.StatusOK,
		},
		{
			desc:      "the proxy cannot serve the version",
			status:    http.StatusNotFound,
			expectErr: "the Go module proxy cannot serve github.com/traefik/plugintest@v2.0.0: 404 Not Found: not found: unknown revision\n",
		},
		{
			desc:      "the version is gone from the proxy",
			status:    http.StatusGone,
			expectErr: "the Go module proxy cannot serve github.com/traefik/plugintest@v2.0.0: 410 Gone: not found: unknown revision\n",
		},
		{
			desc:          "the proxy is down",
			status:        http.StatusBadGateway,
			notPluginProb: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			s := Scrapper{gp: newFakeGoProxy(t, test.status)}

			err := s.checkProxyVersion(t.Context(), "github.com/traefik/plugintest", "v2.0.0")

			switch {
			case test.notPluginProb:
				require.ErrorIs(t, err, errGoProxy)
				assert.True(t, notPluginProblem(err))
			case test.expectErr != "":
				require.EqualError(t, err, test.expectErr)
				assert.False(t, notPluginProblem(err))
			default:
				require.NoError(t, err)
			}
		})
	}
}
