package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/google/go-github/v57/github"
	"github.com/ldez/grignotin/goproxy"
	"golang.org/x/mod/modfile"
)

// errGoProxy is returned when the Go module proxy cannot be read: it is not a plugin problem.
var errGoProxy = errors.New("failed to read the Go module proxy")

// verifyYaegiPlugin checks the plugin at latestVersion, its go.mod read at ref.
func (s *Scrapper) verifyYaegiPlugin(ctx context.Context, repository *github.Repository, latestVersion, ref string, manifest Manifest) (string, []string, error) {
	// Gets module information
	mod, err := s.getModuleInfo(ctx, repository, ref)
	if err != nil {
		return "", nil, err
	}

	pluginName := mod.Module.Mod.Path

	// skip already existing plugin
	prev, err := s.pg.GetByName(ctx, pluginName)
	if err == nil && prev != nil && prev.LatestVersion == latestVersion && prev.Stars == repository.GetStargazersCount() {
		return "", nil, nil
	}

	// Checks module information
	err = checkModuleFile(mod, manifest)
	if err != nil {
		return "", nil, err
	}

	err = checkRepoName(repository, pluginName, manifest)
	if err != nil {
		return "", nil, err
	}

	// plugin-service downloads a public plugin from the Go module proxy.
	// The Go module proxy cannot fetch a private repository: do not send it its name.
	if !repository.GetPrivate() {
		err = s.checkProxyVersion(ctx, pluginName, latestVersion)
		if err != nil {
			return "", nil, err
		}
	}

	// Get versions
	versions, err := s.getVersions(ctx, repository, pluginName)
	if err != nil {
		return "", nil, err
	}

	// The plugin code is not checked.
	return pluginName, versions, nil
}

// checkProxyVersion checks that the Go module proxy can serve module@version.
// For example, it cannot serve a v2.0.0 tag when the module path does not end with /v2.
func (s *Scrapper) checkProxyVersion(ctx context.Context, moduleName, version string) error {
	_, err := s.gp.GetInfoWithContext(ctx, moduleName, version)
	if err == nil {
		return nil
	}

	var apiErr *goproxy.APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusGone) {
		return fmt.Errorf("the Go module proxy cannot serve %s@%s: %s", moduleName, version, apiErr.Message)
	}

	return fmt.Errorf("%w: %w", errGoProxy, err)
}

func (s *Scrapper) getModuleInfo(ctx context.Context, repository *github.Repository, version string) (*modfile.File, error) {
	ctx, span := s.tracer.Start(ctx, "scrapper_getModuleInfo")
	defer span.End()

	opts := &github.RepositoryContentGetOptions{Ref: version}

	contents, _, resp, err := s.gh.Repositories.GetContents(ctx, repository.GetOwner().GetLogin(), repository.GetName(), "go.mod", opts)
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		span.RecordError(fmt.Errorf("missing manifest: %w", err))
		return nil, fmt.Errorf("missing manifest: %w", err)
	}

	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	content, err := contents.GetContent()
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	mod, err := modfile.Parse("go.mod", []byte(content), nil)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return mod, nil
}

func checkRepoName(repository *github.Repository, moduleName string, manifest Manifest) error {
	repoName := path.Join("github.com", repository.GetFullName())

	if !strings.HasPrefix(moduleName, repoName) {
		return fmt.Errorf("unsupported plugin: the module name (%s) doesn't contain the GitHub repository name (%s)", moduleName, repoName)
	}

	if !strings.HasPrefix(manifest.Import, repoName) {
		return fmt.Errorf("unsupported plugin: the import name (%s) doesn't contain the GitHub repository name (%s)", manifest.Import, repoName)
	}

	return nil
}

func checkModuleFile(mod *modfile.File, manifest Manifest) error {
	for _, require := range mod.Require {
		if strings.Contains(require.Mod.Path, "github.com/containous/yaegi") ||
			strings.Contains(require.Mod.Path, "github.com/containous/traefik") ||
			strings.Contains(require.Mod.Path, "github.com/containous/maesh") ||
			strings.Contains(require.Mod.Path, "github.com/traefik/yaegi") ||
			strings.Contains(require.Mod.Path, "github.com/traefik/traefik") ||
			strings.Contains(require.Mod.Path, "github.com/traefik/mesh") {
			return fmt.Errorf("a plugin cannot have a dependence to: %s", require.Mod.Path)
		}
	}

	if !strings.HasPrefix(strings.ReplaceAll(manifest.Import, "-", "_"), strings.ReplaceAll(mod.Module.Mod.Path, "-", "_")) {
		return fmt.Errorf("the import %q must be related to the module name %q", manifest.Import, mod.Module.Mod.Path)
	}

	return nil
}
