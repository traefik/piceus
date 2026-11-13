package core

import (
	"net/http"
	"testing"

	"github.com/google/go-github/v57/github"
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

func TestCheckRepoName(t *testing.T) {
	testCases := []struct {
		desc        string
		repository  *github.Repository
		moduleName  string
		manifest    Manifest
		expectError bool
	}{
		{
			desc: "Fails if repo name is not prefix of module name",
			repository: &github.Repository{
				FullName: new("foo/bar"),
			},
			moduleName: "github.com/something/wrong",
			manifest: Manifest{
				Import: "github.com/foo/bar",
			},
			expectError: true,
		},
		{
			desc: "Fails if repo name is not prefix of manifest import",
			repository: &github.Repository{
				FullName: new("foo/bar"),
			},
			moduleName: "github.com/foo/bar",
			manifest: Manifest{
				Import: "github.com/something/wrong",
			},
			expectError: true,
		},
		{
			desc: "Succeeds",
			repository: &github.Repository{
				FullName: new("foo/bar"),
			},
			moduleName: "github.com/foo/bar",
			manifest: Manifest{
				Import: "github.com/foo/bar",
			},
			expectError: false,
		},
		{
			desc: "Succeeds if repo name contains uppercase characters, but module name and manifest import do not",
			repository: &github.Repository{
				FullName: new("FOO/bar"),
			},
			moduleName: "github.com/foo/bar",
			manifest: Manifest{
				Import: "github.com/foo/bar",
			},
			expectError: false,
		},
		{
			desc: "Succeeds with mixed case in both repo name and module name",
			repository: &github.Repository{
				FullName: new("Foo/Bar"),
			},
			moduleName: "github.com/Foo/Bar",
			manifest: Manifest{
				Import: "github.com/Foo/Bar",
			},
			expectError: false,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			err := checkRepoName(test.repository, test.moduleName, test.manifest)

			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
