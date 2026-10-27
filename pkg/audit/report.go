package audit

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Diff compares two audits of the same catalog, keyed by module@version.
type Diff struct {
	// Regressions were OK and are not anymore, or are missing now.
	Regressions []Change `json:"regressions"`
	// Unchecked were OK and hit a network error now, re-run to confirm.
	Unchecked []Change `json:"unchecked"`
	// BytesChanged are OK on both sides but serve different bytes.
	BytesChanged []Change `json:"bytesChanged"`
	// Fixed were not OK and are now.
	Fixed []Change `json:"fixed"`
	// StillBroken are not OK on either side.
	StillBroken []Change `json:"stillBroken"`
	// Removed were audited before, not OK, and are missing now.
	Removed []string `json:"removed"`
	// Added are missing from the previous audit.
	Added []string `json:"added"`
}

// verdictMissing is the After verdict of an entry missing from the second audit.
const verdictMissing = "missing"

// Change is one entry of the first audit, compared with the second.
type Change struct {
	Key    string `json:"key"`
	Before Result `json:"before"`
	After  Result `json:"after"`
}

// Compare diffs before and after.
func Compare(before, after []Result) Diff {
	prev := make(map[string]Result, len(before))
	for _, r := range before {
		prev[r.Key()] = r
	}

	var d Diff

	seen := map[string]bool{}

	for _, a := range after {
		key := a.Key()
		seen[key] = true

		b, ok := prev[key]
		if !ok {
			d.Added = append(d.Added, key)
			continue
		}

		c := Change{Key: key, Before: b, After: a}

		switch wasOK, isOK := b.Verdict == VerdictOK, a.Verdict == VerdictOK; {
		case wasOK && a.Verdict == VerdictNetworkError:
			d.Unchecked = append(d.Unchecked, c)
		case wasOK && !isOK:
			d.Regressions = append(d.Regressions, c)
		case wasOK && b.ServedHash != a.ServedHash:
			d.BytesChanged = append(d.BytesChanged, c)
		case !wasOK && isOK:
			d.Fixed = append(d.Fixed, c)
		case !wasOK:
			d.StillBroken = append(d.StillBroken, c)
		}
	}

	d.addMissing(before, seen)

	return d
}

// addMissing records the entries of before missing from the second audit.
func (d *Diff) addMissing(before []Result, seen map[string]bool) {
	for _, b := range before {
		if seen[b.Key()] {
			continue
		}

		// prune only removes versions that were not downloadable.
		if b.Verdict == VerdictOK {
			d.Regressions = append(d.Regressions, Change{Key: b.Key(), Before: b, After: Result{Entry: b.Entry, Verdict: verdictMissing}})
			continue
		}

		d.Removed = append(d.Removed, b.Key())
	}
}

// WriteReport writes a markdown summary of an audit.
func WriteReport(w io.Writer, baseURL string, results []Result) {
	byVerdict := map[string][]Result{}
	for _, r := range results {
		byVerdict[r.Verdict] = append(byVerdict[r.Verdict], r)
	}

	var moved []Result

	for _, r := range results {
		if r.Tag != nil && r.Tag.Moved {
			moved = append(moved, r)
		}
	}

	p := printer{w: w}

	p.f("# Plugin catalog audit\n\n")
	p.f("Date: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	p.f("Download `%s/public/download/<module>/<version>`, sha256, then validate with that hash, compared with the recorded hash.\n\n", baseURL)

	p.f("| Result | Entries |\n|---|---:|\n")
	p.f("| Checked | %d |\n", len(results))
	p.f("| OK | %d |\n", len(byVerdict[VerdictOK]))
	p.f("| Hash mismatch | %d |\n", len(byVerdict[VerdictMismatch]))
	p.f("| Rejected by validate | %d |\n", len(byVerdict[VerdictValidateFailed]))
	p.f("| Not downloadable | %d |\n", len(byVerdict[VerdictDownloadError]))
	p.f("| Could not be checked | %d |\n", len(byVerdict[VerdictNetworkError]))
	p.f("| Tag moved after publication | %d |\n\n", len(moved))

	p.hashTable("Hash mismatch", byVerdict[VerdictMismatch])
	p.hashTable("Rejected by validate", byVerdict[VerdictValidateFailed])
	p.statusTable("Not downloadable", byVerdict[VerdictDownloadError])
	p.statusTable("Could not be checked", byVerdict[VerdictNetworkError])

	p.f("## Tag moved after publication (%d)\n\n", len(moved))

	if len(moved) == 0 {
		p.f("None.\n\n")
		return
	}

	p.f("| Module | Version | Verdict | Proxy commit | Tag commit |\n|---|---|---|---|---|\n")

	for _, r := range moved {
		p.f("| `%s` | `%s` | %s | `%s` | `%s` |\n", r.Module, r.Version, r.Verdict, short(r.Tag.ProxyCommit), short(r.Tag.TagCommit))
	}

	p.f("\n")
}

// WriteDiff writes a markdown summary of a Diff.
func WriteDiff(w io.Writer, d Diff) {
	p := printer{w: w}

	p.f("# Plugin catalog audit diff\n\n")
	p.f("| Change | Entries |\n|---|---:|\n")
	p.f("| Regressions | %d |\n", len(d.Regressions))
	p.f("| Unchecked, network error | %d |\n", len(d.Unchecked))
	p.f("| OK, bytes changed | %d |\n", len(d.BytesChanged))
	p.f("| Fixed | %d |\n", len(d.Fixed))
	p.f("| Still broken | %d |\n", len(d.StillBroken))
	p.f("| Removed | %d |\n", len(d.Removed))
	p.f("| Added | %d |\n\n", len(d.Added))

	p.changeTable("Regressions", d.Regressions)
	p.changeTable("Unchecked, network error", d.Unchecked)
	p.changeTable("OK, bytes changed", d.BytesChanged)
	p.changeTable("Fixed", d.Fixed)
	p.changeTable("Still broken", d.StillBroken)
	p.keyList("Removed", d.Removed)
	p.keyList("Added", d.Added)
}

type printer struct {
	w io.Writer
}

func (p printer) f(format string, args ...any) {
	_, _ = fmt.Fprintf(p.w, format, args...)
}

func (p printer) hashTable(title string, rows []Result) {
	p.f("## %s (%d)\n\n", title, len(rows))

	if len(rows) == 0 {
		p.f("None.\n\n")
		return
	}

	p.f("| Module | Version | Runtime | Stars | Listed | Recorded hash | Served hash | Validate | CDN |\n")
	p.f("|---|---|---|---:|---|---|---|---:|---|\n")

	for _, r := range rows {
		p.f("| `%s` | `%s` | %s | %d | %s | `%s` | `%s` | %d | %s |\n",
			r.Module, r.Version, runtime(r), r.Stars, yesNo(r.Listed), r.Hash, r.ServedHash, r.ValidateStatus, dash(r.CacheStatus))
	}

	p.f("\n")
}

func (p printer) statusTable(title string, rows []Result) {
	p.f("## %s (%d)\n\n", title, len(rows))

	if len(rows) == 0 {
		p.f("None.\n\n")
		return
	}

	p.f("| Module | Version | Runtime | Listed | Status | Detail |\n|---|---|---|---|---:|---|\n")

	for _, r := range rows {
		p.f("| `%s` | `%s` | %s | %s | %d | %s |\n", r.Module, r.Version, runtime(r), yesNo(r.Listed), r.DownloadStatus, dash(r.Error))
	}

	p.f("\n")
}

func (p printer) changeTable(title string, rows []Change) {
	p.f("## %s (%d)\n\n", title, len(rows))

	if len(rows) == 0 {
		p.f("None.\n\n")
		return
	}

	p.f("| Entry | Before | After | Served before | Served after |\n|---|---|---|---|---|\n")

	for _, c := range rows {
		p.f("| `%s` | %s | %s | `%s` | `%s` |\n", c.Key, c.Before.Verdict, c.After.Verdict, dash(short(c.Before.ServedHash)), dash(short(c.After.ServedHash)))
	}

	p.f("\n")
}

func (p printer) keyList(title string, keys []string) {
	p.f("## %s (%d)\n\n", title, len(keys))

	if len(keys) == 0 {
		p.f("None.\n\n")
		return
	}

	for _, k := range keys {
		p.f("- `%s`\n", k)
	}

	p.f("\n")
}

func runtime(r Result) string {
	if r.Runtime == "" {
		return "yaegi"
	}

	return r.Runtime
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}

	return "no"
}

func dash(s string) string {
	if s == "" {
		return "-"
	}

	return s
}

func short(s string) string {
	return strings.TrimSpace(s[:min(len(s), 12)])
}
