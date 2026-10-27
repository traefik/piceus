package audit

import (
	"context"
	"slices"
	"sort"

	"github.com/traefik/piceus/pkg/catalog"
	"golang.org/x/mod/semver"
)

// SelectWarmup returns the latest recorded version of the top plugins by stars,
// plugins with dependencies first: they are served from GitHub, the slowest path.
func SelectWarmup(ctx context.Context, t *TagChecker, entries []catalog.Entry, top, concurrency int) []catalog.Entry {
	latest := map[string]catalog.Entry{}

	for _, e := range entries {
		if !e.Listed || e.Hidden || e.Disabled {
			continue
		}

		if cur, ok := latest[e.Module]; !ok || semver.Compare(e.Version, cur.Version) > 0 {
			latest[e.Module] = e
		}
	}

	selected := make([]catalog.Entry, 0, len(latest))
	for _, e := range latest {
		selected = append(selected, e)
	}

	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Stars != selected[j].Stars {
			return selected[i].Stars > selected[j].Stars
		}

		return selected[i].Module < selected[j].Module
	})

	selected = selected[:min(top, len(selected))]

	deps := make([]bool, len(selected))

	forEach(ctx, len(selected), concurrency, func(i int) {
		if !selected[i].IsWasm() {
			deps[i], _ = t.HasDependencies(ctx, selected[i].Module, selected[i].Version)
		}
	})

	var withDeps, others []catalog.Entry

	for i, e := range selected {
		if deps[i] {
			withDeps = append(withDeps, e)
		} else {
			others = append(others, e)
		}
	}

	return slices.Concat(withDeps, others)
}

// Warmup downloads each archive once, so the CDN caches it.
func Warmup(ctx context.Context, c *Client, entries []catalog.Entry, concurrency int) []Result {
	results := make([]Result, len(entries))

	forEach(ctx, len(entries), concurrency, func(i int) {
		results[i] = warmupOne(ctx, c, entries[i])
	})

	return results
}

func warmupOne(ctx context.Context, c *Client, entry catalog.Entry) Result {
	res := Result{Entry: entry}

	dl, err := c.Download(ctx, entry.Module, entry.Version)

	res.DownloadStatus, res.CacheStatus, res.ServedHash, res.Size = dl.Status, dl.CacheStatus, dl.Hash, dl.Size

	switch {
	case err != nil && dl.Status != 0:
		res.Verdict, res.Error = VerdictDownloadError, err.Error()
	case err != nil:
		res.Verdict, res.Error = VerdictNetworkError, err.Error()
	case dl.Hash != entry.Hash:
		res.Verdict = VerdictMismatch
	default:
		res.Verdict = VerdictOK
	}

	return res
}
