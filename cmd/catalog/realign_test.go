package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/traefik/piceus/pkg/audit"
	"github.com/traefik/piceus/pkg/catalog"
)

func TestSkipReason(t *testing.T) {
	entry := catalog.Entry{Module: "github.com/a/b", Version: "v1.0.0"}
	wasm := catalog.Entry{Module: "github.com/a/b", Version: "v1.0.0", Runtime: "wasm"}
	moved := &audit.TagCheck{Moved: true}

	testCases := []struct {
		desc        string
		result      audit.Result
		only        []string
		includeWasm bool
		skipped     bool
	}{
		{desc: "mismatch", result: audit.Result{Entry: entry, Verdict: audit.VerdictMismatch}},
		{desc: "ok", result: audit.Result{Entry: entry, Verdict: audit.VerdictOK}, skipped: true},
		{desc: "not downloadable", result: audit.Result{Entry: entry, Verdict: audit.VerdictDownloadError}, skipped: true},
		{desc: "wasm", result: audit.Result{Entry: wasm, Verdict: audit.VerdictMismatch}, skipped: true},
		{desc: "wasm included", result: audit.Result{Entry: wasm, Verdict: audit.VerdictMismatch}, includeWasm: true},
		{desc: "tag moved", result: audit.Result{Entry: entry, Verdict: audit.VerdictMismatch, Tag: moved}, skipped: true},
		{desc: "tag moved, named", result: audit.Result{Entry: entry, Verdict: audit.VerdictMismatch, Tag: moved}, only: []string{entry.Key()}},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			reason := skipReason(test.result, test.only, test.includeWasm)

			assert.Equal(t, test.skipped, reason != "", reason)
		})
	}
}
