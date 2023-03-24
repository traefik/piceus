package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/go-github/v45/github"
	"github.com/ldez/grignotin/goproxy"
	"github.com/mitchellh/mapstructure"
	"github.com/pelletier/go-toml"
	"github.com/rs/zerolog/log"
	pfile "github.com/traefik/paerser/file"
	"github.com/traefik/piceus/internal/plugin"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

// PrivateModeEnv "private" behavior (uses GitHub instead of GoProxy).
const PrivateModeEnv = "PICEUS_PRIVATE_MODE"

const manifestFile = ".traefik.yml"

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

If you believe there is a problem with the Analyzer or this issue is the result of a false positive, please contact [us](https://community.traefik.io/).
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
func NewScrapper(gh *github.Client, gp *goproxy.Client, pgClient pluginClient, sources Sources) *Scrapper {
	return &Scrapper{
		gh:      gh,
		gp:      gp,
		pg:      pgClient,
		sources: sources,

		// TODO improve blacklist storage
		blacklist: map[string]struct{}{
			"containous/plugintestxxx":     {},
			"esenac/traefik-custom-router": {}, // The repo doesn't allow issues https://github.com/esenac/traefik-custom-router
		},
		skipNewCall: map[string]struct{}{
			"github.com/negasus/traefik-plugin-ip2location": {},
		},

		tracer: otel.Tracer("scrapper"),
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

	if contains(reposWithExistingIssue, repository.GetFullName()) {
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
	ctx, span := s.tracer.Start(ctx, "scrapper_process_"+*repository.Name)
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

	// Gets module information

	mod, err := s.getModuleInfo(ctx, repository, latestVersion)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	moduleName := mod.Module.Mod.Path

	// skip already existing plugin

	prev, err := s.pg.GetByName(ctx, moduleName)
	if err == nil && prev != nil && prev.LatestVersion == latestVersion && prev.Stars == repository.GetStargazersCount() {
		return nil, nil
	}

	// Checks module information

	err = checkModuleFile(mod, manifest)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	err = checkRepoName(repository, moduleName, manifest)
	if err != nil {
		return nil, err
	}

	// Get versions

	versions, err := s.getVersions(ctx, repository, moduleName)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Creates temp GOPATH

	gop, err := os.MkdirTemp("", "traefik-plugin-gop")
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to create temp GOPATH: %w", err)
	}

	defer func() { _ = os.RemoveAll(gop) }()

	// Get sources

	hash, err := s.sources.Get(ctx, repository, gop, module.Version{Path: moduleName, Version: latestVersion})
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get sources: %w", err)
	}

	// Check Yaegi interface
	err = s.yaegiCheck(manifest, gop, moduleName)
	if err != nil {
		return nil, fmt.Errorf("failed to run the plugin with Yaegi: %w", err)
	}

	snippets, err := createSnippets(repository, manifest)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return &plugin.Plugin{
		Name:          moduleName,
		DisplayName:   manifest.DisplayName,
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

func (s *Scrapper) getModuleInfo(ctx context.Context, repository *github.Repository, version string) (*modfile.File, error) {
	ctx, span := s.tracer.Start(ctx, "scrapper_getModuleInfo")
	defer span.End()

	opts := &github.RepositoryContentGetOptions{Ref: version}

	contents, _, resp, err := s.gh.Repositories.GetContents(ctx, repository.GetOwner().GetLogin(), repository.GetName(), "go.mod", opts)
	if resp != nil && resp.StatusCode == 404 {
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

func (s *Scrapper) loadManifest(ctx context.Context, repository *github.Repository, version string) (Manifest, error) {
	ctx, span := s.tracer.Start(ctx, "scrapper_loadManifest")
	defer span.End()

	opts := &github.RepositoryContentGetOptions{Ref: version}

	contents, _, resp, err := s.gh.Repositories.GetContents(ctx, repository.GetOwner().GetLogin(), repository.GetName(), manifestFile, opts)
	if resp != nil && resp.StatusCode == 404 {
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
	case typeMiddleware, typeProvider:
		// noop
	default:
		return Manifest{}, fmt.Errorf("unsupported type: %s", m.Type)
	}

	if m.Import == "" {
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
		span.RecordError(fmt.Errorf("missing tag/version"))
		return "", errors.New("missing tag/version")
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
		span.RecordError(fmt.Errorf("missing tag/version"))
		return nil, errors.New("missing tags/versions")
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

	var result []string
	for _, tag := range tags {
		if tag.GetName() != "" {
			result = append(result, tag.GetName())
		}
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

func (s *Scrapper) yaegiCheck(manifest Manifest, goPath, moduleName string) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic from yaegi: %v", rec)
		}
	}()

	tearDown := dropSensitiveEnvVars()
	defer tearDown()

	switch manifest.Type {
	case typeMiddleware:
		_, skip := s.skipNewCall[moduleName]
		return yaegiMiddlewareCheck(goPath, manifest, skip)

	case typeProvider:
		// TODO yaegi check for provider
		return nil

	default:
		return fmt.Errorf("unsupported type: %s", manifest.Type)
	}
}

func yaegiMiddlewareCheck(goPath string, manifest Manifest, skipNew bool) error {
	middlewareName := "test"

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	i := interp.New(interp.Options{GoPath: goPath})
	if err := i.Use(stdlib.Symbols); err != nil {
		return fmt.Errorf("load of stdlib symbols: %w", err)
	}

	_, err := i.EvalWithContext(ctx, fmt.Sprintf(`import %q`, manifest.Import))
	if err != nil {
		return fmt.Errorf("the load of the plugin takes too much time(%s), or an error, inside the plugin, occurs during the load: %w", timeout, err)
	}

	basePkg := manifest.BasePkg
	if basePkg == "" {
		basePkg = path.Base(manifest.Import)
		basePkg = strings.ReplaceAll(basePkg, "-", "_")
	}

	vConfig, err := i.EvalWithContext(ctx, basePkg+`.CreateConfig()`)
	if err != nil {
		return fmt.Errorf("failed to eval `CreateConfig` function: %w", err)
	}

	err = decodeConfig(vConfig, manifest.TestData)
	if err != nil {
		return err
	}

	fnNew, err := i.EvalWithContext(ctx, basePkg+`.New`)
	if err != nil {
		return fmt.Errorf("failed to eval `New` function: %w", err)
	}

	err = checkFunctionNewSignature(fnNew, vConfig)
	if err != nil {
		return fmt.Errorf("the signature of the function `New` is invalid: %w", err)
	}

	if !skipNew {
		args := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(next), vConfig, reflect.ValueOf(middlewareName)}
		results, err := safeFnCall(fnNew, args)
		if err != nil {
			return fmt.Errorf("the function `New` of %s produce a panic: %w", middlewareName, err)
		}

		if len(results) > 1 && results[1].Interface() != nil {
			return fmt.Errorf("failed to create a new plugin instance: %w", results[1].Interface().(error))
		}

		_, ok := results[0].Interface().(http.Handler)
		if !ok {
			return fmt.Errorf("invalid handler type: %T", results[0].Interface())
		}
	}

	return nil
}

func safeFnCall(fn reflect.Value, args []reflect.Value) (result []reflect.Value, errCall error) {
	defer func() {
		if err := recover(); err != nil {
			errCall = fmt.Errorf("panic during the call of the function: %v", err)
		}
	}()

	result = fn.Call(args)

	return
}

func decodeConfig(vConfig reflect.Value, testData interface{}) error {
	cfg := &mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.StringToSliceHookFunc(","),
		WeaklyTypedInput: true,
		Result:           vConfig.Interface(),
	}

	decoder, err := mapstructure.NewDecoder(cfg)
	if err != nil {
		return fmt.Errorf("plugin: failed to create configuration decoder: %w", err)
	}

	err = decoder.Decode(testData)
	if err != nil {
		return fmt.Errorf("plugin: failed to decode configuration: %w", err)
	}

	return nil
}

func checkFunctionNewSignature(fnNew, vConfig reflect.Value) error {
	// check in types

	if fnNew.Type().NumIn() != 4 {
		return fmt.Errorf("invalid input arguments: got %d arguments expected %d", fnNew.Type().NumIn(), 4)
	}

	if !fnNew.Type().In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
		return errors.New("invalid input arguments: the 1st argument must have the type context.Context")
	}

	if !fnNew.Type().In(1).Implements(reflect.TypeOf((*http.Handler)(nil)).Elem()) {
		return errors.New("invalid input arguments: the 2nd argument must have the type http.Handler")
	}

	if !fnNew.Type().In(2).AssignableTo(vConfig.Type()) {
		return errors.New("invalid input arguments: the 3rd argument must have the same type as the Config structure")
	}

	if fnNew.Type().In(3).Kind() != reflect.String {
		return errors.New("invalid input arguments: the 4th argument must have the type string")
	}

	// check out types

	if fnNew.Type().NumOut() != 2 {
		return fmt.Errorf("invalid output arguments: got %d arguments expected %d", fnNew.Type().NumOut(), 2)
	}

	if !fnNew.Type().Out(0).Implements(reflect.TypeOf((*http.Handler)(nil)).Elem()) {
		return errors.New("invalid input arguments: the 1st argument must have the type http.Handler")
	}

	if !fnNew.Type().Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return errors.New("invalid input arguments: the 2nd argument must have the type error")
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

func dropSensitiveEnvVars() func() {
	bckEnviron := make(map[string]string)

	for _, ev := range os.Environ() {
		pair := strings.SplitN(ev, "=", 2)

		key := strings.ToLower(pair[0])
		if strings.Contains(key, "token") ||
			strings.Contains(key, "password") ||
			strings.Contains(key, "username") ||
			strings.Contains(key, "_url") ||
			strings.Contains(key, "_host") ||
			strings.Contains(key, "_port") {
			bckEnviron[pair[0]] = pair[1]
			_ = os.Unsetenv(pair[0])
		}
	}

	return func() {
		for k, v := range bckEnviron {
			_ = os.Setenv(k, v)
		}
	}
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

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}

	return false
}
