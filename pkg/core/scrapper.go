package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/go-github/v57/github"
	"github.com/ldez/grignotin/goproxy"
	"github.com/pelletier/go-toml"
	"github.com/rs/zerolog/log"
	pfile "github.com/traefik/paerser/file"
	"github.com/traefik/piceus/internal/plugin"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"
)

// PrivateModeEnv "private" behavior (uses GitHub instead of GoProxy).
const PrivateModeEnv = "PICEUS_PRIVATE_MODE"

const manifestFile = ".traefik.yml"

const wasmRuntime = "wasm"

const (
	typeMiddleware = "middleware"
	typeProvider   = "provider"
)

const (
	// searchQuery the query used to search plugins on GitHub.
	// https://help.github.com/en/github/searching-for-information-on-github/searching-for-repositories
	searchQuery = "topic:traefik-plugin language:Go archived:false is:public"

	// searchQueryIssues the query used to search issues opened by the bot account.
	searchQueryIssues = "is:open is:issue is:public author:traefiker"
)

const (
	oldIssueTitle = "[Traefik Pilot] Traefik Plugin Analyzer has detected a problem." // must be keep forever.
	issueTitle    = "[Traefik Plugin Catalog] Plugin Analyzer has detected a problem."
	issueContent  = `The plugin was not imported into Traefik Plugin Catalog.

Cause:
` + "```" + `
%v
` + "```" + `
Traefik Plugin Analyzer will restart when you will close this issue.

If you believe there is a problem with the Analyzer or this issue is the result of a false positive, please fill an issue on [piceus](https://github.com/traefik/piceus) repository.
`
)

type pluginClient interface {
	Create(ctx context.Context, p plugin.Plugin) error
	Update(ctx context.Context, p plugin.Plugin) error
	GetByName(ctx context.Context, name string) (*plugin.Plugin, error)
}

// Scrapper the plugins scrapper.
type Scrapper struct {
	gh          *github.Client
	gp          *goproxy.Client
	pg          pluginClient
	sources     Sources
	blacklist   map[string]struct{}
	skipNewCall map[string]struct{} // temporary approach
	tracer      trace.Tracer
}

// NewScrapper creates a new Scrapper instance.
func NewScrapper(gh *github.Client, gp *goproxy.Client, pgClient pluginClient, sources Sources, tracer trace.Tracer) *Scrapper {
	return &Scrapper{
		gh:      gh,
		gp:      gp,
		pg:      pgClient,
		sources: sources,

		// TODO improve blacklist storage
		blacklist: map[string]struct{}{
			"containous/plugintestxxx":                {},
			"esenac/traefik-custom-router":            {}, // Doesn't allow issues
			"alexdelprete/traefik-oidc-relying-party": {},
			"FinalCAD/TraefikGrpcWebPlugin":           {}, // Crash piceus.
			"deas/teectl":                             {}, // Not a plugin
			"GDGVIT/securum-exire":                    {}, // Not a plugin
		},
		skipNewCall: map[string]struct{}{
			"github.com/negasus/traefik-plugin-ip2location": {},
		},

		tracer: tracer,
	}
}

// Run runs the scrapper.
func (s *Scrapper) Run(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "scrapper_run")
	defer span.End()

	reposWithExistingIssue, err := s.searchReposWithExistingIssue(ctx)
	if err != nil {
		span.RecordError(err)
		return err
	}

	repositories, err := s.search(ctx)
	if err != nil {
		span.RecordError(err)
		return err
	}

	for _, repository := range repositories {
		logger := log.With().Str("repo_name", repository.GetFullName()).Logger()
		logger.Debug().Msg("Processing repository")

		if s.isSkipped(logger.WithContext(ctx), reposWithExistingIssue, repository) {
			continue
		}

		data, err := s.process(logger.WithContext(ctx), repository)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to import repository")

			errResp := &github.ErrorResponse{}
			if errors.As(err, &errResp) && errResp.Response.StatusCode >= http.StatusInternalServerError {
				span.RecordError(err)
				continue
			}

			issue := &github.IssueRequest{
				Title: github.String(issueTitle),
				Body:  github.String(safeIssueBody(err)),
			}
			_, _, err = s.gh.Issues.Create(ctx, repository.GetOwner().GetLogin(), repository.GetName(), issue)
			if err != nil {
				span.RecordError(err)
				logger.Error().Err(err).Msg("Failed to create issue")
			}

			continue
		}

		err = s.store(logger.WithContext(ctx), data)
		if err != nil {
			span.RecordError(err)
			logger.Error().Err(err).Msg("Failed to store plugin")
		}
	}

	return nil
}

func (s *Scrapper) isSkipped(ctx context.Context, reposWithExistingIssue []string, repository *github.Repository) bool {
	if _, ok := s.blacklist[repository.GetFullName()]; ok {
		return true
	}

	if slices.Contains(reposWithExistingIssue, repository.GetFullName()) {
		log.Ctx(ctx).Debug().Msg("The issue is still opened.")
		return true
	}

	return false
}

func (s *Scrapper) searchReposWithExistingIssue(ctx context.Context) ([]string, error) {
	opts := &github.SearchOptions{
		Sort:        "updated",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var all []string

	for {
		issues, resp, err := s.gh.Search.Issues(ctx, searchQueryIssues, opts)
		if err != nil {
			return nil, err
		}

		for _, issue := range issues.Issues {
			if issue.GetTitle() == oldIssueTitle || issue.GetTitle() == issueTitle {
				// Creates the fullname of the repository.
				all = append(all, strings.TrimPrefix(issue.GetRepositoryURL(), "https://api.github.com/repos/"))
			}
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return all, nil
}

func (s *Scrapper) search(ctx context.Context) ([]*github.Repository, error) {
	ctx, span := s.tracer.Start(ctx, "scrapper_search")
	defer span.End()

	opts := &github.SearchOptions{
		Sort:        "updated",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var all []*github.Repository

	for {
		repositories, resp, err := s.gh.Search.Repositories(ctx, searchQuery, opts)
		if err != nil {
			span.RecordError(err)
			return nil, err
		}

		all = append(all, repositories.Repositories...)
		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return all, nil
}

func (s *Scrapper) process(ctx context.Context, repository *github.Repository) (*plugin.Plugin, error) {
	ctx, span := s.tracer.Start(ctx, "scrapper_process_"+repository.GetName())
	defer span.End()

	latestVersion, err := s.getLatestTag(ctx, repository)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get the latest tag: %w", err)
	}

	// Gets readme

	readme, err := s.loadReadme(ctx, repository, latestVersion)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to load readme: %w", err)
	}

	// Gets manifestFile

	manifest, err := s.loadManifest(ctx, repository, latestVersion)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	var versions []string
	var pluginName string

	switch manifest.Runtime {
	case wasmRuntime:
		pluginName, versions, err = s.verifyWASMPlugin(ctx, repository, latestVersion, manifest)
		if err != nil {
			span.RecordError(err)
			return nil, err
		}

		if pluginName == "" {
			return nil, nil
		}

	default:
		pluginName, versions, err = s.verifyYaegiPlugin(ctx, repository, latestVersion, manifest)
		if err != nil {
			span.RecordError(err)
			return nil, err
		}

		if pluginName == "" {
			return nil, nil
		}
	}

	snippets, err := createSnippets(repository, manifest)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return &plugin.Plugin{
		Name:          pluginName,
		DisplayName:   manifest.DisplayName,
		Runtime:       manifest.Runtime,
		Author:        repository.GetOwner().GetLogin(),
		RepoName:      repository.GetName(),
		Type:          manifest.Type,
		Import:        manifest.Import,
		Compatibility: manifest.Compatibility,
		Summary:       manifest.Summary,
		IconURL:       parseImageURL(repository, latestVersion, manifest.IconPath),
		BannerURL:     parseImageURL(repository, latestVersion, manifest.BannerPath),
		Readme:        readme,
		LatestVersion: latestVersion,
		Versions:      versions,
		Stars:         repository.GetStargazersCount(),
		Snippet:       snippets,
	}, nil
}

func (s *Scrapper) loadManifest(ctx context.Context, repository *github.Repository, version string) (Manifest, error) {
	ctx, span := s.tracer.Start(ctx, "scrapper_loadManifest")
	defer span.End()

	opts := &github.RepositoryContentGetOptions{Ref: version}

	contents, _, resp, err := s.gh.Repositories.GetContents(ctx, repository.GetOwner().GetLogin(), repository.GetName(), manifestFile, opts)
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		span.RecordError(fmt.Errorf("missing manifest: %w", err))
		return Manifest{}, fmt.Errorf("missing manifest: %w", err)
	}

	if err != nil {
		span.RecordError(err)
		return Manifest{}, fmt.Errorf("failed to get manifest: %w", err)
	}

	content, err := contents.GetContent()
	if err != nil {
		span.RecordError(err)
		return Manifest{}, fmt.Errorf("failed to get manifest content: %w", err)
	}

	return s.loadManifestContent(content)
}

func (s *Scrapper) loadManifestContent(content string) (Manifest, error) {
	var m Manifest
	err := yaml.Unmarshal([]byte(content), &m)
	if err != nil {
		return Manifest{}, fmt.Errorf("failed to read manifest content: %w", err)
	}

	if len(m.TestData) > 0 {
		var mp Manifest
		err = pfile.DecodeContent(content, ".yaml", &mp)
		if err != nil {
			return Manifest{}, fmt.Errorf("failed to read testdata from manifest: %w", err)
		}

		m.TestData = mp.TestData
	}

	switch m.Type {
	case typeMiddleware:
	// noop
	case typeProvider:
		if m.Runtime == wasmRuntime {
			return Manifest{}, fmt.Errorf("unsupported type for WASM plugin: %s", m.Type)
		}
	default:
		return Manifest{}, fmt.Errorf("unsupported type: %s", m.Type)
	}

	if m.Runtime != wasmRuntime && m.Import == "" {
		return Manifest{}, errors.New("missing import")
	}

	if m.DisplayName == "" {
		return Manifest{}, errors.New("missing DisplayName")
	}

	if m.Summary == "" {
		return Manifest{}, errors.New("missing Summary")
	}

	if m.TestData == nil {
		return Manifest{}, errors.New("missing TestData")
	}

	return m, nil
}

func (s *Scrapper) loadReadme(ctx context.Context, repository *github.Repository, version string) (string, error) {
	ctx, span := s.tracer.Start(ctx, "scrapper_loadReadme")
	defer span.End()

	opts := &github.RepositoryContentGetOptions{Ref: version}

	readme, _, err := s.gh.Repositories.GetReadme(ctx, repository.GetOwner().GetLogin(), repository.GetName(), opts)
	if err != nil {
		span.RecordError(err)
		return "", fmt.Errorf("failed to get the readme file: %w", err)
	}

	content, err := readme.GetContent()
	if err != nil {
		span.RecordError(err)
		return "", fmt.Errorf("failed to get manifest content: %w", err)
	}

	return content, nil
}

func (s *Scrapper) getLatestTag(ctx context.Context, repository *github.Repository) (string, error) {
	ctx, span := s.tracer.Start(ctx, "scrapper_getLatestTag")
	defer span.End()

	tags, err := s.getTags(ctx, repository)
	if err != nil {
		span.RecordError(err)
		return "", err
	}

	if len(tags) == 0 {
		err := errors.New("missing tag/version")
		span.RecordError(err)
		return "", err
	}

	return tags[0], nil
}

func (s *Scrapper) getVersions(ctx context.Context, repository *github.Repository, moduleName string) ([]string, error) {
	ctx, span := s.tracer.Start(ctx, "scrapper_getVersions")
	defer span.End()

	var versions []string
	var err error

	if _, ok := os.LookupEnv(PrivateModeEnv); ok {
		versions, err = s.getTags(ctx, repository)
	} else {
		versions, err = s.gp.GetVersions(moduleName)
	}

	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	if len(versions) == 0 {
		err = errors.New("missing tag/version")
		span.RecordError(err)
		return nil, err
	}

	return versions, err
}

func (s *Scrapper) getTags(ctx context.Context, repository *github.Repository) ([]string, error) {
	ctx, span := s.tracer.Start(ctx, "scrapper_getTags")
	defer span.End()

	tags, _, err := s.gh.Repositories.ListTags(ctx, repository.GetOwner().GetLogin(), repository.GetName(), nil)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get versions: %w", err)
	}

	expSemver := regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

	var result []string
	for _, tag := range tags {
		name := tag.GetName()
		if name == "" {
			continue
		}

		if !expSemver.MatchString(name) {
			return nil, fmt.Errorf("invalid tag: %s (this tag must be removed, see https://semver.org)", name)
		}

		result = append(result, name)
	}

	return result, nil
}

func (s *Scrapper) store(ctx context.Context, data *plugin.Plugin) error {
	if data == nil {
		return nil
	}

	logger := log.Ctx(ctx).With().Str("module_name", data.Name).Logger()

	ctx, span := s.tracer.Start(ctx, "scrapper_store_"+data.Name)
	defer span.End()

	prev, err := s.pg.GetByName(ctx, data.Name)
	if err != nil {
		span.RecordError(err)

		var notFoundError *plugin.APIError
		if errors.As(err, &notFoundError) && notFoundError.StatusCode == http.StatusNotFound {
			logger.Debug().Err(err).Msg("fallback")

			err = s.pg.Create(ctx, *data)
			if err != nil {
				return err
			}

			logger.Info().Msg("Stored")
			return nil
		}

		return fmt.Errorf("API error on %s: %w", data.Name, err)
	}

	if cmp.Equal(data, prev, cmpopts.IgnoreFields(plugin.Plugin{}, "ID", "CreatedAt")) {
		return nil
	}

	data.ID = prev.ID
	data.CreatedAt = prev.CreatedAt

	err = s.pg.Update(ctx, *data)
	if err != nil {
		span.RecordError(err)
		return err
	}

	if prev.LatestVersion != data.LatestVersion {
		logger.Info().Str("latest_version", data.LatestVersion).Msg("Updated")
	}

	return nil
}

func createSnippets(repository *github.Repository, manifest Manifest) (map[string]interface{}, error) {
	switch manifest.Type {
	case typeMiddleware:
		return createMiddlewareSnippets(repository, manifest.TestData)
	case typeProvider:
		return createProviderSnippets(repository, manifest.TestData)
	default:
		return nil, fmt.Errorf("unsupported type: %s", manifest.Type)
	}
}

func createMiddlewareSnippets(repository *github.Repository, testData map[string]interface{}) (map[string]interface{}, error) {
	snip := map[string]interface{}{
		"http": map[string]interface{}{
			"middlewares": map[string]interface{}{
				"my-" + repository.GetName(): map[string]interface{}{
					"plugin": map[string]interface{}{
						repository.GetName(): testData,
					},
				},
			},
		},
	}

	yamlSnip, err := yaml.Marshal(snip)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall (YAML): %w", err)
	}

	tomlSnip, err := toml.Marshal(snip)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall (YAML): %w", err)
	}

	k8s := map[string]interface{}{
		"apiVersion": "traefik.containo.us/v1alpha1",
		"kind":       "Middleware",
		"metadata": map[string]interface{}{
			"name":      "my-" + repository.GetName(),
			"namespace": "my-namespace",
		},
		"spec": map[string]interface{}{
			"plugin": map[string]interface{}{
				repository.GetName(): testData,
			},
		},
	}
	k8sSnip, err := yaml.Marshal(k8s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall (YAML): %w", err)
	}

	return map[string]interface{}{
		"toml": string(tomlSnip),
		"yaml": string(yamlSnip),
		"k8s":  string(k8sSnip),
	}, nil
}

func createProviderSnippets(repository *github.Repository, testData map[string]interface{}) (map[string]interface{}, error) {
	snip := map[string]interface{}{
		"providers": map[string]interface{}{
			"plugin": map[string]interface{}{
				repository.GetName(): testData,
			},
		},
	}

	yamlSnip, err := yaml.Marshal(snip)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall (YAML): %w", err)
	}

	tomlSnip, err := toml.Marshal(snip)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall (YAML): %w", err)
	}

	return map[string]interface{}{
		"toml": string(tomlSnip),
		"yaml": string(yamlSnip),
	}, nil
}

func parseImageURL(repository *github.Repository, latestVersion, imgPath string) string {
	if imgPath == "" {
		return ""
	}

	img, err := url.Parse(imgPath)
	if err != nil {
		return ""
	}

	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s", repository.GetOwner().GetLogin(), repository.GetName())
	if strings.HasPrefix(imgPath, rawURL) {
		return imgPath
	}

	rawURL = fmt.Sprintf("https://github.com/%s/%s/raw/", repository.GetOwner().GetLogin(), repository.GetName())
	if strings.HasPrefix(imgPath, rawURL) {
		return imgPath
	}

	if img.Host != "" {
		return ""
	}

	baseURL, err := url.Parse(repository.GetHTMLURL())
	if err != nil {
		return ""
	}

	baseURL.Host = "raw.githubusercontent.com"

	pictURL, err := baseURL.Parse(path.Join(baseURL.Path, latestVersion, path.Clean(img.Path)))
	if err != nil {
		return ""
	}

	return pictURL.String()
}

func safeIssueBody(err error) string {
	msgBody := err.Error()

	var repKeys []string
	for _, ev := range os.Environ() {
		pair := strings.SplitN(ev, "=", 2)

		key := strings.ToLower(pair[0])
		if strings.Contains(key, "token") ||
			strings.Contains(key, "password") ||
			strings.Contains(key, "username") {
			repKeys = append(repKeys, pair[0], pair[1])
		}

		if strings.Contains(key, "_url") ||
			strings.Contains(key, "_host") ||
			strings.Contains(key, "_port") {
			repKeys = append(repKeys, pair[0])

			if len(pair[1]) > 5 {
				repKeys = append(repKeys, pair[1])
			}
		}
	}

	sort.Slice(repKeys, func(i, j int) bool {
		return len(repKeys[i]) > len(repKeys[j])
	})

	replacements := make([]string, 0, len(repKeys))
	for _, key := range repKeys {
		replacements = append(replacements, key, "xxx")
	}

	replacer := strings.NewReplacer(replacements...)
	msgBody = replacer.Replace(msgBody)

	return fmt.Sprintf(issueContent, msgBody)
}
