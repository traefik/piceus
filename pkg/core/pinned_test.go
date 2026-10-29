package core

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-github/v57/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/piceus/pkg/audit"
	"go.opentelemetry.io/otel"
	"golang.org/x/mod/module"
)

func TestScrapper_yaegiRef(t *testing.T) {
	answer := func(results ...error) (PinnedCommit, *int) {
		calls := 0

		return func(_ context.Context, mod, version string) (string, error) {
			assert.Equal(t, "github.com/o/r", mod)
			assert.Equal(t, "v1.0.0", version)

			err := results[min(calls, len(results)-1)]
			calls++

			if err != nil {
				return "", err
			}

			return "abc123", nil
		}, &calls
	}

	testCases := []struct {
		desc     string
		repo     string // r has the pinned commit, gone does not, down fails the check
		private  bool
		results  []error // one per call to the proxy, the last one repeats; nil gives the pinned commit
		noProxy  bool
		ref      string
		calls    int
		wantErr  bool
		proxyErr bool
	}{
		{desc: "pinned", results: []error{nil}, ref: "abc123", calls: 1},
		{desc: "not pinned", results: []error{audit.ErrNotPinned}, ref: "v1.0.0", calls: 1},
		{desc: "proxy back after a hiccup", results: []error{errors.New("502"), nil}, ref: "abc123", calls: 2},
		{desc: "proxy down", results: []error{errors.New("502")}, calls: 3, wantErr: true, proxyErr: true},
		{desc: "private repository", private: true, results: []error{nil}, ref: "v1.0.0"},
		{desc: "no proxy", noProxy: true, ref: "v1.0.0"},
		{desc: "pinned commit gone", repo: "gone", results: []error{nil}, ref: "v1.0.0", calls: 1},
		{desc: "commit check down", repo: "down", results: []error{nil}, calls: 1, wantErr: true},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			repo := cmp.Or(test.repo, "r")

			files := map[string]string{
				"go.mod@v1.0.0": "module github.com/o/r\n",
				"go.mod@abc123": "module github.com/o/r\n",
			}

			s := Scrapper{gh: newFakeGitHub(t, files, nil), tracer: otel.GetTracerProvider().Tracer("test")}

			var calls *int
			if !test.noProxy {
				s.pinnedCommit, calls = answer(test.results...)
			}

			repository := &github.Repository{Name: new(repo), Owner: &github.User{Login: new("o")}, Private: new(test.private)}

			ref, err := s.yaegiRef(t.Context(), repository, "v1.0.0")
			if calls != nil {
				assert.Equal(t, test.calls, *calls)
			}

			if test.wantErr {
				require.Error(t, err)
				assert.Equal(t, test.proxyErr, errors.Is(err, errPinnedCommit))

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.ref, ref)
		})
	}
}

func TestScrapper_process_movedTag(t *testing.T) {
	// The versions come from the GitHub tags, not from the Go proxy.
	t.Setenv(PrivateModeEnv, "true")

	fixtureDir := filepath.Join("fixtures", "simple")

	manifest, err := os.ReadFile(filepath.Join(fixtureDir, manifestFile))
	require.NoError(t, err)

	goMod, err := os.ReadFile(filepath.Join(fixtureDir, "go.mod"))
	require.NoError(t, err)

	// The tag moved to a commit whose manifest does not match the pinned code.
	moved := map[string]string{
		manifestFile + "@v1.0.0": "displayName: Moved\ntype: middleware\nimport: github.com/traefik/plugintestsimple/moved\nsummary: Moved.\ntestData: {}\n",
		"go.mod@v1.0.0":          string(goMod),
		manifestFile + "@abc123": string(manifest) + "\ntestData: {}\n",
		"go.mod@abc123":          string(goMod),
	}

	// The pinned commit is gone: plugin-service serves the tag.
	gone := map[string]string{
		manifestFile + "@v1.0.0": string(manifest) + "\ntestData: {}\n",
		"go.mod@v1.0.0":          string(goMod),
	}

	testCases := []struct {
		desc    string
		files   map[string]string
		readmes map[string]string
		ref     string
		readme  string
	}{
		{desc: "pinned commit", files: moved, readmes: map[string]string{"v1.0.0": "# Moved", "abc123": "# Pinned"}, ref: "abc123", readme: "# Pinned"},
		{desc: "readme added after publication", files: moved, readmes: map[string]string{"v1.0.0": "# Moved"}, ref: "abc123", readme: "# Moved"},
		{desc: "pinned commit gone", files: gone, readmes: map[string]string{"v1.0.0": "# Tag"}, ref: "v1.0.0", readme: "# Tag"},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			srcs := &refSources{src: fixtureDir}

			s := NewScrapper(newFakeGitHub(t, test.files, test.readmes), nil, &mockPluginClient{}, true, srcs,
				func(context.Context, string, string) (string, error) { return "abc123", nil },
				newTestEvaluator(t), nil, nil)

			repository := &github.Repository{
				Name:     new("plugintestsimple"),
				FullName: new("traefik/plugintestsimple"),
				Owner:    &github.User{Login: new("traefik")},
			}

			p, err := s.process(t.Context(), repository)
			require.NoError(t, err)
			require.NotNil(t, p)

			assert.Equal(t, test.ref, srcs.ref)
			assert.Equal(t, "Plugin Without Unsafe", p.DisplayName)
			assert.Equal(t, test.readme, p.Readme)
			assert.Equal(t, "v1.0.0", p.LatestVersion)
		})
	}
}

// refSources records the ref the sources are read from.
type refSources struct {
	LocalSources

	ref string
}

func (s *refSources) Get(ctx context.Context, repository *github.Repository, gop string, mod module.Version, ref string) error {
	s.ref = ref

	return s.LocalSources.Get(ctx, repository, gop, mod, ref)
}

// newFakeGitHub serves the tag v1.0.0, files keyed by path@ref and readmes keyed by ref.
// A commit exists when a file is keyed by it, except in the repository gone; the repository down fails the commit check.
func newFakeGitHub(t *testing.T, files, readmes map[string]string) *github.Client {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/{owner}/{repo}/tags", func(rw http.ResponseWriter, _ *http.Request) {
		writeJSON(t, rw, http.StatusOK, []*github.RepositoryTag{{Name: new("v1.0.0")}})
	})
	mux.HandleFunc("GET /repos/{owner}/{repo}/readme", func(rw http.ResponseWriter, req *http.Request) {
		content, ok := readmes[req.URL.Query().Get("ref")]
		if !ok {
			rw.WriteHeader(http.StatusNotFound)
			return
		}

		writeJSON(t, rw, http.StatusOK, fileContent(content))
	})
	mux.HandleFunc("GET /repos/{owner}/{repo}/commits/{sha}", func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case req.PathValue("repo") == "down":
			rw.WriteHeader(http.StatusServiceUnavailable)
		case req.PathValue("repo") != "gone" && slices.ContainsFunc(slices.Collect(maps.Keys(files)), func(k string) bool { return strings.HasSuffix(k, "@"+req.PathValue("sha")) }):
			rw.WriteHeader(http.StatusNotModified)
		default:
			rw.WriteHeader(http.StatusUnprocessableEntity)
		}
	})
	mux.HandleFunc("GET /repos/{owner}/{repo}/contents/{path...}", func(rw http.ResponseWriter, req *http.Request) {
		content, ok := files[req.PathValue("path")+"@"+req.URL.Query().Get("ref")]
		if !ok {
			rw.WriteHeader(http.StatusNotFound)
			return
		}

		writeJSON(t, rw, http.StatusOK, fileContent(content))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := github.NewClient(nil)

	var err error
	client.BaseURL, err = url.Parse(server.URL + "/")
	require.NoError(t, err)

	return client
}
