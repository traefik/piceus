package audit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v57/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/piceus/pkg/catalog"
)

func TestGithubRef(t *testing.T) {
	testCases := []struct {
		module string
		owner  string
		repo   string
		tag    string
	}{
		{module: "github.com/o/r", owner: "o", repo: "r", tag: "v1.2.3"},
		{module: "github.com/o/r/v2", owner: "o", repo: "r", tag: "v1.2.3"},
		{module: "github.com/o/r/sub", owner: "o", repo: "r", tag: "sub/v1.2.3"},
		{module: "github.com/o/r/sub/v3", owner: "o", repo: "r", tag: "sub/v1.2.3"},
	}

	for _, test := range testCases {
		t.Run(test.module, func(t *testing.T) {
			owner, repo, tag := githubRef(test.module, "v1.2.3")

			assert.Equal(t, test.owner, owner)
			assert.Equal(t, test.repo, repo)
			assert.Equal(t, test.tag, tag)
		})
	}
}

func TestIsGitHubModule(t *testing.T) {
	assert.True(t, IsGitHubModule("github.com/o/r"))
	assert.True(t, IsGitHubModule("github.com/o/r/sub/v2"))
	assert.False(t, IsGitHubModule("github.com/o"))
	assert.False(t, IsGitHubModule("github.com/o/"))
	assert.False(t, IsGitHubModule("gitlab.com/o/r"))
}

func TestCheckTags(t *testing.T) {
	proxy := http.NewServeMux()
	proxy.HandleFunc("/github.com/o/moved/@v/v1.0.0.mod", func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(rw, "module github.com/o/moved\n\nrequire github.com/x/y v1.0.0\n")
	})
	proxy.HandleFunc("/github.com/o/moved/@v/v1.0.0.info", func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(rw, `{"Version":"v1.0.0","Origin":{"VCS":"git","Hash":"aaa"}}`)
	})
	proxy.HandleFunc("/github.com/o/stable/@v/v1.0.0.mod", func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(rw, "module github.com/o/stable\n\nrequire github.com/x/y v1.0.0\n")
	})
	proxy.HandleFunc("/github.com/o/stable/@v/v1.0.0.info", func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(rw, `{"Version":"v1.0.0","Origin":{"VCS":"git","Hash":"bbb"}}`)
	})
	proxy.HandleFunc("/github.com/o/nodeps/@v/v1.0.0.mod", func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(rw, "module github.com/o/nodeps\n")
	})

	proxySrv := httptest.NewServer(proxy)
	t.Cleanup(proxySrv.Close)

	gh := http.NewServeMux()
	gh.HandleFunc("/repos/o/moved/commits/v1.0.0", func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(rw, "ccc")
	})
	gh.HandleFunc("/repos/o/stable/commits/v1.0.0", func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(rw, "bbb")
	})

	ghSrv := httptest.NewServer(gh)
	t.Cleanup(ghSrv.Close)

	ghClient := github.NewClient(nil)
	ghClient.BaseURL, _ = url.Parse(ghSrv.URL + "/")

	results := []Result{
		{Module: "github.com/o/moved", Version: "v1.0.0"},
		{Module: "github.com/o/stable", Version: "v1.0.0"},
		{Module: "github.com/o/nodeps", Version: "v1.0.0"},
		{Module: "github.com/o/wasm", Version: "v1.0.0", Runtime: "wasm"},
	}

	CheckTags(t.Context(), NewTagChecker(proxySrv.URL, ghClient), results, 2)

	require.NotNil(t, results[0].Tag)
	assert.Equal(t, &TagCheck{ProxyCommit: "aaa", TagCommit: "ccc", Moved: true}, results[0].Tag)
	assert.Equal(t, &TagCheck{ProxyCommit: "bbb", TagCommit: "bbb"}, results[1].Tag)
	assert.Equal(t, &TagCheck{NoDependencies: true}, results[2].Tag)
	assert.Nil(t, results[3].Tag)
}

func TestGone(t *testing.T) {
	const withDeps = "module m\n\nrequire github.com/x/y v1.0.0\n"

	proxy := http.NewServeMux()
	proxy.HandleFunc("/github.com/o/noproxy/@v/v1.0.0.mod", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusGone)
	})
	proxy.HandleFunc("/github.com/o/nodeps/@v/v1.0.0.mod", func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(rw, "module m\n")
	})
	proxy.HandleFunc("/github.com/o/proxydown/@v/v1.0.0.mod", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusBadGateway)
	})

	// Tag deleted, pinned commit still there: plugin-service serves the pinned commit.
	// Repository deleted: the pinned commit is gone too.
	for _, name := range []string{"tagdeleted", "repodeleted", "nopin", "ghdown", "infodown"} {
		proxy.HandleFunc("/github.com/o/"+name+"/@v/v1.0.0.mod", func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(rw, withDeps)
		})
	}

	for _, name := range []string{"tagdeleted", "repodeleted", "ghdown"} {
		proxy.HandleFunc("/github.com/o/"+name+"/@v/v1.0.0.info", func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(rw, `{"Version":"v1.0.0","Origin":{"Hash":"pinned"}}`)
		})
	}

	proxy.HandleFunc("/github.com/o/infodown/@v/v1.0.0.info", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusBadGateway)
	})

	proxySrv := httptest.NewServer(proxy)
	t.Cleanup(proxySrv.Close)

	gh := http.NewServeMux()
	gh.HandleFunc("/repos/o/tagdeleted/commits/pinned", func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(rw, "pinned")
	})
	gh.HandleFunc("/repos/o/repodeleted/commits/pinned", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	})
	gh.HandleFunc("/repos/o/nopin/commits/v1.0.0", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusUnprocessableEntity)
	})
	gh.HandleFunc("/repos/o/infodown/commits/v1.0.0", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusUnprocessableEntity)
	})
	gh.HandleFunc("/repos/o/ghdown/commits/pinned", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusServiceUnavailable)
	})
	gh.HandleFunc("/repos/o/wasm/commits/v1.0.0", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	})

	ghSrv := httptest.NewServer(gh)
	t.Cleanup(ghSrv.Close)

	ghClient := github.NewClient(nil)
	ghClient.BaseURL, _ = url.Parse(ghSrv.URL + "/")

	checker := NewTagChecker(proxySrv.URL, ghClient)

	testCases := []struct {
		name    string
		runtime string
		gone    bool
		wantErr bool
	}{
		{name: "noproxy", gone: true},
		{name: "nodeps"},
		{name: "proxydown", wantErr: true},
		{name: "tagdeleted"},
		{name: "repodeleted", gone: true},
		{name: "nopin", gone: true},
		{name: "ghdown", wantErr: true},
		// No answer on the pinned commit: the tag being gone proves nothing.
		{name: "infodown", wantErr: true},
		{name: "wasm", runtime: "wasm", gone: true},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			gone, err := checker.Gone(t.Context(), catalog.Entry{Module: "github.com/o/" + test.name, Version: "v1.0.0", Runtime: test.runtime})

			if test.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.gone, gone)
		})
	}
}
