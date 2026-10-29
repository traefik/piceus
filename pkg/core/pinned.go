package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/traefik/piceus/pkg/audit"
)

// errPinnedCommit is returned when the Go module proxy cannot be read: it is not a plugin problem.
var errPinnedCommit = errors.New("failed to get the commit pinned by the Go module proxy")

// PinnedCommit returns the commit the Go module proxy pinned for module@version, or audit.ErrNotPinned.
type PinnedCommit func(ctx context.Context, module, version string) (string, error)

// yaegiRef returns the ref to read the Yaegi plugin from: the commit the Go module proxy pinned at publication,
// that plugin-service serves once the tag moved, or the tag when the proxy pinned none.
func (s *Scrapper) yaegiRef(ctx context.Context, repository *github.Repository, version string) (string, error) {
	// The Go module proxy cannot fetch a private repository: do not send it its name.
	if s.pinnedCommit == nil || repository.GetPrivate() {
		return version, nil
	}

	mod, err := s.getModuleInfo(ctx, repository, version)
	if err != nil {
		return "", err
	}

	if mod.Module == nil {
		return version, nil
	}

	var pinned string

	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * s.retryWait):
			}
		}

		pinned, err = s.pinnedCommit(ctx, mod.Module.Mod.Path, version)
		if err == nil || errors.Is(err, audit.ErrNotPinned) {
			break
		}
	}

	switch {
	case errors.Is(err, audit.ErrNotPinned):
		return version, nil
	case err != nil:
		return "", fmt.Errorf("%w: %w", errPinnedCommit, err)
	}

	// plugin-service serves the tag once the pinned commit is gone from GitHub.
	gone, err := s.commitGone(ctx, repository, pinned)
	if err != nil {
		return "", err
	}

	if gone {
		return version, nil
	}

	return pinned, nil
}

// commitGone reports whether GitHub no longer has the commit sha.
func (s *Scrapper) commitGone(ctx context.Context, repository *github.Repository, sha string) (bool, error) {
	// A conditional request answered 304 is not counted in the GitHub rate limit.
	_, resp, err := s.gh.Repositories.GetCommitSHA1(ctx, repository.GetOwner().GetLogin(), repository.GetName(), sha, sha)

	switch {
	case err == nil, resp != nil && resp.StatusCode == http.StatusNotModified:
		return false, nil
	case resp != nil && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnprocessableEntity):
		return true, nil
	default:
		return false, fmt.Errorf("check commit %s: %w", sha, err)
	}
}
