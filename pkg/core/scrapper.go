package core

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"reflect"
	"strings"
	"time"

	"github.com/containous/piceus/internal/plugin"
	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/go-github/v32/github"
	"github.com/ldez/grignotin/goproxy"
	"github.com/mitchellh/mapstructure"
	"github.com/pelletier/go-toml"
	"github.com/rs/zerolog/log"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

// PrivateModeEnv "private" behavior (uses GitHub instead of GoProxy).
const PrivateModeEnv = "PICEUS_PRIVATE_MODE"

const manifestFile = ".traefik.yml"

// searchQuery the query used to search plugins on GitHub.
// https://help.github.com/en/github/searching-for-information-on-github/searching-for-repositories
const searchQuery = "topic:traefik-plugin language:Go archived:false is:public"

const (
	issueTitle   = "[Traefik Pilot] Traefik Plugin Analyzer has detected a problem."
	issueContent = `The plugin was not imported into Traefik Pilot.

Cause:
` + "```" + `
%v
` + "```" + `
Traefik Plugin Analyzer will restart when you will close this issue.

If you believe there is a problem with the Analyzer or this issue is the result of a false positive, please contact [us](https://community.containo.us/).
`
)

type pluginClient interface {
	Create(p plugin.Plugin) error
	Update(p plugin.Plugin) error
	GetByName(name string) (*plugin.Plugin, error)
}

// Scrapper the plugins scrapper.
type Scrapper struct {
	gh          *github.Client
	gp          *goproxy.Client
	pg          pluginClient
	sources     Sources
	blacklist   map[string]struct{}
	skipNewCall map[string]struct{} // temporary approach
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
			"containous/plugintestxxx": {},
		},
		skipNewCall: map[string]struct{}{
			"github.com/negasus/traefik-plugin-ip2location": {},
		},
	}
}

// Run runs the scrapper.
func (s *Scrapper) Run(ctx context.Context) error {
	repositories, err := s.search(ctx)
	if err != nil {
		return err
	}

	for _, repository := range repositories {
		if s.isSkipped(ctx, repository) {
			continue
		}

		log.Info().Msg(repository.GetHTMLURL())

		data, err := s.process(ctx, repository)
		if err != nil {
			log.Error().Msgf("%s: failed to import repository: %v", repository.GetFullName(), err.Error())

			issue := &github.IssueRequest{
				Title: github.String(issueTitle),
				Body:  github.String(fmt.Sprintf(issueContent, err)),
			}
			_, _, err = s.gh.Issues.Create(ctx, repository.GetOwner().GetLogin(), repository.GetName(), issue)
			if err != nil {
				log.Error().Msgf("%s: failed to create issue: %v", repository.GetFullName(), err.Error())
			}

			continue
		}

		err = s.store(data)
		if err != nil {
			log.Error().Msgf("%s: failed to store plugin: %v", repository.GetFullName(), err)
		}
	}

	return nil
}

func (s *Scrapper) isSkipped(ctx context.Context, repository *github.Repository) bool {
	if _, ok := s.blacklist[repository.GetFullName()]; ok {
		return true
	}

	if s.hasIssue(ctx, repository) {
		log.Info().Msgf("%s: the issue is still opened.", repository.GetFullName())
		return true
	}

	return false
}

func (s *Scrapper) hasIssue(ctx context.Context, repository *github.Repository) bool {
	user, _, err := s.gh.Users.Get(ctx, "")
	if err != nil {
		log.Error().Msgf("%s: failed to get current GitHub user: %v", repository.GetFullName(), err)
		return false
	}

	opts := &github.IssueListByRepoOptions{
		State:   "open",
		Creator: user.GetLogin(),
	}

	issues, _, err := s.gh.Issues.ListByRepo(ctx, repository.GetOwner().GetLogin(), repository.GetName(), opts)
	if err != nil {
		log.Error().Msgf("%s: failed to list issues on repo: %v", repository.GetFullName(), err)
		return false
	}

	for _, issue := range issues {
		if issue.GetTitle() == issueTitle {
			return true
		}
	}

	return false
}

func (s *Scrapper) search(ctx context.Context) ([]*github.Repository, error) {
	opts := &github.SearchOptions{
		Sort:        "updated",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var all []*github.Repository

	for {
		repositories, resp, err := s.gh.Search.Repositories(ctx, searchQuery, opts)
		if err != nil {
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
	latestVersion, err := s.getLatestTag(ctx, repository)
	if err != nil {
		return nil, fmt.Errorf("failed to get the latest tag: %w", err)
	}

	// Gets readme

	readme, err := s.loadReadme(ctx, repository, latestVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get readme: %w", err)
	}

	// Gets manifestFile

	manifest, err := s.loadManifest(ctx, repository, latestVersion)
	if err != nil {
		return nil, err
	}

	// Checks module information

	mod, err := s.getModuleInfo(ctx, repository, latestVersion)
	if err != nil {
		return nil, err
	}

	for _, require := range mod.Require {
		if strings.Contains(require.Mod.Path, "github.com/containous/yaegi") ||
			strings.Contains(require.Mod.Path, "github.com/containous/traefik") ||
			strings.Contains(require.Mod.Path, "github.com/containous/maesh") {
			return nil, fmt.Errorf("a plugin cannot have a dependence to: %v", require.Mod.Path)
		}
	}

	moduleName := mod.Module.Mod.Path

	// Checks consistency

	if !strings.HasPrefix(strings.ReplaceAll(manifest.Import, "-", "_"), strings.ReplaceAll(moduleName, "-", "_")) {
		return nil, fmt.Errorf("the import %q must be related to the module name %q", manifest.Import, moduleName)
	}

	// Gets versions

	versions, err := s.getVersions(ctx, repository, moduleName)
	if err != nil {
		return nil, err
	}

	// Creates temp GOPATH

	gop, err := ioutil.TempDir("", "pilot-gop")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp GOPATH: %w", err)
	}

	defer func() { _ = os.RemoveAll(gop) }()

	// Gets sources

	err = s.sources.Get(ctx, repository, gop, module.Version{Path: moduleName, Version: latestVersion})
	if err != nil {
		return nil, fmt.Errorf("failed to get sources: %w", err)
	}

	// Check Yaegi interface

	if manifest.Type == "middleware" {
		_, skip := s.skipNewCall[moduleName]

		err = yaegiCheck(gop, manifest, skip)
		if err != nil {
			return nil, fmt.Errorf("failed to run with Yaegi: %w", err)
		}
	}

	snippets, err := createSnippets(repository, manifest.TestData)
	if err != nil {
		return nil, err
	}

	return &plugin.Plugin{
		Name:          moduleName,
		DisplayName:   manifest.DisplayName,
		Author:        repository.GetOwner().GetLogin(),
		Type:          manifest.Type,
		Import:        manifest.Import,
		Compatibility: manifest.Compatibility,
		Summary:       manifest.Summary,
		Readme:        readme,
		LatestVersion: latestVersion,
		Versions:      versions,
		Stars:         repository.GetStargazersCount(),
		Snippet:       snippets,
	}, nil
}

func createSnippets(repository *github.Repository, testData map[string]interface{}) (map[string]interface{}, error) {
	snip := map[string]interface{}{
		"middlewares": map[string]interface{}{
			"my-" + repository.GetName(): map[string]interface{}{
				"plugin": map[string]interface{}{
					repository.GetName(): testData,
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

	return map[string]interface{}{
		"toml": string(tomlSnip),
		"yaml": string(yamlSnip),
	}, nil
}

func (s *Scrapper) getModuleInfo(ctx context.Context, repository *github.Repository, version string) (*modfile.File, error) {
	opts := &github.RepositoryContentGetOptions{Ref: version}

	contents, _, resp, err := s.gh.Repositories.GetContents(ctx, repository.GetOwner().GetLogin(), repository.GetName(), "go.mod", opts)
	if resp != nil && resp.StatusCode == 404 {
		return nil, fmt.Errorf("missing manifest: %w", err)
	}

	if err != nil {
		return nil, err
	}

	content, err := contents.GetContent()
	if err != nil {
		return nil, err
	}

	mod, err := modfile.Parse("go.mod", []byte(content), nil)
	if err != nil {
		return nil, err
	}

	return mod, nil
}

func (s *Scrapper) loadManifest(ctx context.Context, repository *github.Repository, version string) (Manifest, error) {
	opts := &github.RepositoryContentGetOptions{Ref: version}

	contents, _, resp, err := s.gh.Repositories.GetContents(ctx, repository.GetOwner().GetLogin(), repository.GetName(), manifestFile, opts)
	if resp != nil && resp.StatusCode == 404 {
		return Manifest{}, fmt.Errorf("missing manifest: %w", err)
	}

	if err != nil {
		return Manifest{}, fmt.Errorf("failed to get manifest: %w", err)
	}

	content, err := contents.GetContent()
	if err != nil {
		return Manifest{}, fmt.Errorf("failed to get manifest content: %w", err)
	}

	m := Manifest{}
	err = yaml.Unmarshal([]byte(content), &m)
	if err != nil {
		return Manifest{}, fmt.Errorf("failed to read manifest content: %w", err)
	}

	if m.Type != "middleware" {
		return Manifest{}, errors.New("unsupported type")
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
	opts := &github.RepositoryContentGetOptions{Ref: version}

	readme, _, err := s.gh.Repositories.GetReadme(ctx, repository.GetOwner().GetLogin(), repository.GetName(), opts)
	if err != nil {
		return "", fmt.Errorf("failed to get readme: %w", err)
	}

	content, err := readme.GetContent()
	if err != nil {
		return "", fmt.Errorf("failed to get manifest content: %w", err)
	}

	return content, nil
}

func (s *Scrapper) getLatestTag(ctx context.Context, repository *github.Repository) (string, error) {
	tags, err := s.getTags(ctx, repository)
	if err != nil {
		return "", err
	}

	if len(tags) == 0 {
		return "", errors.New("missing tag/version")
	}

	return tags[0], nil
}

func (s *Scrapper) getVersions(ctx context.Context, repository *github.Repository, moduleName string) ([]string, error) {
	var versions []string
	var err error

	if _, ok := os.LookupEnv(PrivateModeEnv); ok {
		versions, err = s.getTags(ctx, repository)
	} else {
		versions, err = s.gp.GetVersions(moduleName)
	}

	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, errors.New("missing tags/versions")
	}

	return versions, err
}

func (s *Scrapper) getTags(ctx context.Context, repository *github.Repository) ([]string, error) {
	tags, _, err := s.gh.Repositories.ListTags(ctx, repository.GetOwner().GetLogin(), repository.GetName(), nil)
	if err != nil {
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

func (s *Scrapper) store(data *plugin.Plugin) error {
	prev, err := s.pg.GetByName(data.Name)
	if err != nil {
		log.Error().Msg(err.Error())

		err = s.pg.Create(*data)
		if err != nil {
			return err
		}

		log.Info().Msgf("Stored: %v", data.Name)
		return nil
	}

	if cmp.Equal(data, prev, cmpopts.IgnoreFields(plugin.Plugin{}, "ID", "CreatedAt")) {
		return nil
	}

	data.ID = prev.ID
	data.CreatedAt = prev.CreatedAt

	err = s.pg.Update(*data)
	if err != nil {
		return err
	}

	log.Info().Msgf("Updated: %v", data.Name)
	return nil
}

func yaegiCheck(goPath string, manifest Manifest, skipNew bool) error {
	middlewareName := "test"

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	i := interp.New(interp.Options{GoPath: goPath})
	i.Use(stdlib.Symbols)

	_, err := i.EvalWithContext(ctx, fmt.Sprintf(`import "%s"`, manifest.Import))
	if err != nil {
		return fmt.Errorf("plugin: failed to import plugin code: %w", err)
	}

	basePkg := manifest.BasePkg
	if basePkg == "" {
		basePkg = path.Base(manifest.Import)
		basePkg = strings.ReplaceAll(basePkg, "-", "_")
	}

	vConfig, err := i.EvalWithContext(ctx, basePkg+`.CreateConfig()`)
	if err != nil {
		return fmt.Errorf("plugin: failed to eval CreateConfig: %w", err)
	}

	err = decodeConfig(vConfig, manifest.TestData)
	if err != nil {
		return err
	}

	fnNew, err := i.EvalWithContext(ctx, basePkg+`.New`)
	if err != nil {
		return fmt.Errorf("plugin: failed to eval New: %w", err)
	}

	err = checkFunctionNewSignature(fnNew, vConfig)
	if err != nil {
		return fmt.Errorf("the signature of the function `New` is invalid: %w", err)
	}

	if !skipNew {
		args := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(next), vConfig, reflect.ValueOf(middlewareName)}
		results := fnNew.Call(args)

		if len(results) > 1 && results[1].Interface() != nil {
			return fmt.Errorf("plugin: failed to create a new plugin instance: %w", results[1].Interface().(error))
		}

		_, ok := results[0].Interface().(http.Handler)
		if !ok {
			return fmt.Errorf("plugin: invalid handler type: %T", results[0].Interface())
		}
	}

	return nil
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
