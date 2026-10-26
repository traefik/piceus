package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/piceus/pkg/catalog"
)

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// newPluginService serves archives and validates hashes like plugin-service.
// recorded is what validate compares with, keyed by module@version.
func newPluginService(t *testing.T, archives, recorded map[string]string, statuses map[string]int) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var calls atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("/public/download/", func(rw http.ResponseWriter, req *http.Request) {
		calls.Add(1)

		key := keyOf(strings.TrimPrefix(req.URL.Path, "/public/download/"))

		if status, ok := statuses[key]; ok {
			rw.WriteHeader(status)
			return
		}

		_, _ = rw.Write([]byte(archives[key]))
	})
	mux.HandleFunc("/public/validate/", func(rw http.ResponseWriter, req *http.Request) {
		key := keyOf(strings.TrimPrefix(req.URL.Path, "/public/validate/"))

		if recorded[key] == req.Header.Get(hashHeader) {
			rw.WriteHeader(http.StatusOK)
			return
		}

		rw.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv, &calls
}

func keyOf(p string) string {
	i := strings.LastIndex(p, "/")
	return p[:i] + "@" + p[i+1:]
}

func TestRun(t *testing.T) {
	archives := map[string]string{
		"github.com/a/ok@v1.0.0":       "ok",
		"github.com/a/drift@v1.0.0":    "new bytes",
		"github.com/a/rejected@v1.0.0": "rejected",
	}

	recorded := map[string]string{
		"github.com/a/ok@v1.0.0":       sum("ok"),
		"github.com/a/drift@v1.0.0":    sum("old bytes"),
		"github.com/a/rejected@v1.0.0": "something else",
	}

	statuses := map[string]int{
		"github.com/a/gone@v1.0.0":   http.StatusNotFound,
		"github.com/a/broken@v1.0.0": http.StatusInternalServerError,
	}

	srv, calls := newPluginService(t, archives, recorded, statuses)

	entries := []catalog.Entry{
		{Module: "github.com/a/ok", Version: "v1.0.0", Hash: sum("ok")},
		{Module: "github.com/a/drift", Version: "v1.0.0", Hash: sum("old bytes")},
		// The catalog and validate disagree: validate is the source of truth for Traefik.
		{Module: "github.com/a/rejected", Version: "v1.0.0", Hash: sum("rejected")},
		{Module: "github.com/a/gone", Version: "v1.0.0", Hash: "x"},
		{Module: "github.com/a/broken", Version: "v1.0.0", Hash: "x"},
	}

	c := NewClient(srv.URL, "", "")
	c.RetryWait = time.Millisecond

	results := Run(t.Context(), c, entries, 2)
	require.Len(t, results, len(entries))

	verdicts := map[string]string{}
	for _, r := range results {
		verdicts[r.Key()] = r.Verdict
	}

	assert.Equal(t, map[string]string{
		"github.com/a/ok@v1.0.0":       VerdictOK,
		"github.com/a/drift@v1.0.0":    VerdictMismatch,
		"github.com/a/rejected@v1.0.0": VerdictValidateFailed,
		"github.com/a/gone@v1.0.0":     VerdictDownloadError,
		"github.com/a/broken@v1.0.0":   VerdictDownloadError,
	}, verdicts)

	assert.Equal(t, sum("new bytes"), results[1].ServedHash)
	assert.Equal(t, http.StatusInternalServerError, results[4].DownloadStatus)

	// 4 downloads, plus 2 retries for the 500.
	assert.Equal(t, int64(len(entries)+2), calls.Load())
}

func TestDownload_basicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		user, pass, ok := req.BasicAuth()
		if !ok || user != "u" || pass != "p" {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}

		_, _ = rw.Write([]byte("archive"))
	}))
	t.Cleanup(srv.Close)

	dl, err := NewClient(srv.URL, "u", "p").Download(t.Context(), "github.com/a/b", "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, sum("archive"), dl.Hash)

	dl, err = NewClient(srv.URL, "", "").Download(t.Context(), "github.com/a/b", "v1.0.0")
	require.Error(t, err)
	assert.Equal(t, http.StatusUnauthorized, dl.Status)
}

func TestCompare(t *testing.T) {
	entry := func(name string) catalog.Entry {
		return catalog.Entry{Module: "github.com/a/" + name, Version: "v1.0.0"}
	}

	before := []Result{
		{Entry: entry("same"), Verdict: VerdictOK, ServedHash: "1"},
		{Entry: entry("regressed"), Verdict: VerdictOK, ServedHash: "1"},
		{Entry: entry("bytes"), Verdict: VerdictOK, ServedHash: "1"},
		{Entry: entry("fixed"), Verdict: VerdictMismatch, ServedHash: "1"},
		{Entry: entry("broken"), Verdict: VerdictDownloadError},
		{Entry: entry("removed"), Verdict: VerdictOK},
		{Entry: entry("unchecked"), Verdict: VerdictOK, ServedHash: "1"},
	}

	after := []Result{
		{Entry: entry("unchecked"), Verdict: VerdictNetworkError},
		{Entry: entry("same"), Verdict: VerdictOK, ServedHash: "1"},
		{Entry: entry("regressed"), Verdict: VerdictMismatch, ServedHash: "2"},
		{Entry: entry("bytes"), Verdict: VerdictOK, ServedHash: "2"},
		{Entry: entry("fixed"), Verdict: VerdictOK, ServedHash: "1"},
		{Entry: entry("broken"), Verdict: VerdictDownloadError},
		{Entry: entry("added"), Verdict: VerdictOK},
	}

	d := Compare(before, after)

	keys := func(changes []Change) []string {
		var k []string
		for _, c := range changes {
			k = append(k, strings.TrimPrefix(c.Key, "github.com/a/"))
		}

		return k
	}

	assert.Equal(t, []string{"regressed@v1.0.0"}, keys(d.Regressions))
	assert.Equal(t, []string{"unchecked@v1.0.0"}, keys(d.Unchecked))
	assert.Equal(t, []string{"bytes@v1.0.0"}, keys(d.BytesChanged))
	assert.Equal(t, []string{"fixed@v1.0.0"}, keys(d.Fixed))
	assert.Equal(t, []string{"broken@v1.0.0"}, keys(d.StillBroken))
	assert.Equal(t, []string{"github.com/a/removed@v1.0.0"}, d.Removed)
	assert.Equal(t, []string{"github.com/a/added@v1.0.0"}, d.Added)
}
