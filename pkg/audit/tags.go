package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/traefik/piceus/pkg/catalog"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

var majorSuffix = regexp.MustCompile(`^v\d+$`)

// errNotPinned the Go module proxy does not know which commit the version was published from.
var errNotPinned = errors.New("no pinned commit in the proxy info")

// TagCheck compares the commit a tag points at today with the one the Go module proxy pinned at publication.
type TagCheck struct {
	ProxyCommit string `json:"proxyCommit,omitempty"`
	TagCommit   string `json:"tagCommit,omitempty"`
	Moved       bool   `json:"moved"`
	Error       string `json:"error,omitempty"`
}

// TagChecker reads the Go module proxy and GitHub.
type TagChecker struct {
	GoProxyURL string
	HTTP       *http.Client
	GitHub     *github.Client
}

// NewTagChecker creates a TagChecker using the Go module proxy at goProxyURL.
func NewTagChecker(goProxyURL string, gh *github.Client) *TagChecker {
	return &TagChecker{
		GoProxyURL: strings.TrimSuffix(goProxyURL, "/"),
		HTTP:       &http.Client{Timeout: time.Minute},
		GitHub:     gh,
	}
}

// CheckTags fills Result.Tag for the yaegi plugins with dependencies:
// plugin-service serves them from a GitHub zipball, the others come from the Go module proxy, already pinned.
func CheckTags(ctx context.Context, t *TagChecker, results []Result, concurrency int) {
	forEach(ctx, len(results), concurrency, func(i int) {
		r := &results[i]
		if r.IsWasm() || !strings.HasPrefix(r.Module, "github.com/") {
			return
		}

		deps, err := t.HasDependencies(ctx, r.Module, r.Version)
		if err != nil {
			r.Tag = &TagCheck{Error: err.Error()}
			return
		}

		if deps {
			r.Tag = t.Check(ctx, r.Module, r.Version)
		}
	})
}

// HasDependencies reports whether the go.mod of module@version requires other modules.
func (t *TagChecker) HasDependencies(ctx context.Context, mod, version string) (bool, error) {
	data, err := t.proxyGet(ctx, mod, version, ".mod")
	if err != nil {
		return false, err
	}

	file, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return false, fmt.Errorf("parse go.mod: %w", err)
	}

	return len(file.Require) > 0, nil
}

// Check compares the tag commit on GitHub with the Origin.Hash of the Go module proxy.
func (t *TagChecker) Check(ctx context.Context, mod, version string) *TagCheck {
	check := &TagCheck{}

	proxyCommit, err := t.ProxyCommit(ctx, mod, version)
	if err != nil {
		check.Error = err.Error()
		return check
	}

	check.ProxyCommit = proxyCommit

	owner, repo, tag := githubRef(mod, version)

	check.TagCommit, _, err = t.GitHub.Repositories.GetCommitSHA1(ctx, owner, repo, tag, "")
	if err != nil {
		check.Error = fmt.Sprintf("tag commit: %v", err)
		return check
	}

	check.Moved = check.TagCommit != check.ProxyCommit

	return check
}

// ProxyCommit returns the commit pinned by the Go module proxy for module@version.
func (t *TagChecker) ProxyCommit(ctx context.Context, mod, version string) (string, error) {
	data, err := t.proxyGet(ctx, mod, version, ".info")

	var pErr *proxyError
	if errors.As(err, &pErr) && (pErr.status == http.StatusNotFound || pErr.status == http.StatusGone) {
		return "", errNotPinned
	}

	if err != nil {
		return "", err
	}

	var info struct {
		Origin *struct {
			Hash string `json:"Hash"` //nolint:tagliatelle // Go module proxy protocol.
		} `json:"Origin"` //nolint:tagliatelle // Go module proxy protocol.
	}

	if err = json.Unmarshal(data, &info); err != nil {
		return "", fmt.Errorf("decode info: %w", err)
	}

	if info.Origin == nil || info.Origin.Hash == "" {
		return "", errNotPinned
	}

	return info.Origin.Hash, nil
}

// Gone reports whether module@version can no longer be fetched upstream, whatever plugin-service answers:
// the Go module proxy has no go.mod for it, or the GitHub ref plugin-service downloads it from does not exist.
// That ref is the commit pinned by the Go module proxy, the tag when there is none or for wasm plugins.
func (t *TagChecker) Gone(ctx context.Context, entry catalog.Entry) (bool, error) {
	var pinned string

	if !entry.IsWasm() {
		deps, err := t.HasDependencies(ctx, entry.Module, entry.Version)

		var pErr *proxyError

		switch {
		case errors.As(err, &pErr) && (pErr.status == http.StatusNotFound || pErr.status == http.StatusGone):
			// What the proxy answers for a version it cannot serve.
			return true, nil
		case err != nil:
			return false, err
		case !deps:
			// Served from the Go module proxy, which still has it.
			return false, nil
		}

		// Without a pinned commit plugin-service serves the tag; any other error leaves the answer unknown.
		pinned, err = t.ProxyCommit(ctx, entry.Module, entry.Version)
		if err != nil && !errors.Is(err, errNotPinned) {
			return false, err
		}
	}

	if !strings.HasPrefix(entry.Module, "github.com/") {
		return false, nil
	}

	owner, repo, ref := githubRef(entry.Module, entry.Version)
	if pinned != "" {
		ref = pinned
	}

	_, resp, err := t.GitHub.Repositories.GetCommitSHA1(ctx, owner, repo, ref, "")
	if err == nil {
		return false, nil
	}

	// 404: repository deleted, 422: no such ref. A renamed repository answers a redirect, it is not gone.
	if resp != nil && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnprocessableEntity) {
		return true, nil
	}

	return false, err
}

func (t *TagChecker) proxyGet(ctx context.Context, mod, version, ext string) ([]byte, error) {
	escMod, err := module.EscapePath(mod)
	if err != nil {
		return nil, err
	}

	escVersion, err := module.EscapeVersion(version)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s/@v/%s%s", t.GoProxyURL, escMod, escVersion, ext), http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := t.HTTP.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &proxyError{ext: ext, status: resp.StatusCode}
	}

	return io.ReadAll(resp.Body)
}

type proxyError struct {
	ext    string
	status int
}

func (e *proxyError) Error() string {
	return fmt.Sprintf("go proxy %s returned %d", e.ext, e.status)
}

// githubRef returns the repository and tag of a module version hosted on GitHub.
// A module in a subdirectory is tagged <dir>/<version>; a /vN suffix is not a directory.
func githubRef(mod, version string) (string, string, string) {
	parts := strings.Split(mod, "/")
	owner, repo := parts[1], parts[2]

	dir := parts[3:]
	if len(dir) > 0 && majorSuffix.MatchString(dir[len(dir)-1]) {
		dir = dir[:len(dir)-1]
	}

	if len(dir) == 0 {
		return owner, repo, version
	}

	return owner, repo, strings.Join(dir, "/") + "/" + version
}
