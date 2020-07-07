package core

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"reflect"
	"strings"
	"time"

	"github.com/containous/piceus/internal/plugin"
	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
	"github.com/google/go-github/v32/github"
	"github.com/ldez/grignotin/goproxy"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

// PrivateModeEnv "private" behavior (uses GitHub instead of GoProxy).
const PrivateModeEnv = "PICEUS_PRIVATE_MODE"

const manifestFile = ".traefik.yml"

// searchQuery the query used to search plugins on GitHub.
// TODO const searchQuery = "topic:traefik-plugin language:Go archived:false is:public"
// https://help.github.com/en/github/searching-for-information-on-github/searching-for-repositories
const searchQuery = "topic:traefik-plugin language:Go archived:false is:private"

const (
	issueTitle   = "[Traefik Pilot] Traefik Plugin Analyzer has detected a problem."
	issueContent = `The plugin was not imported into Traefik Pilot.

Cause: %v

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
	gh        *github.Client
	gp        *goproxy.Client
	pg        pluginClient
	sources   Sources
	blacklist map[string]struct{}
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

		log.Println(repository.GetHTMLURL())

		data, err := s.process(ctx, repository)
		if err != nil {
			log.Printf("import failure: %s: %v: %s", repository.GetFullName(), err, time.Now())

			issue := &github.IssueRequest{
				Title: github.String(issueTitle),
				Body:  github.String(fmt.Sprintf(issueContent, err)),
			}
			_, _, err = s.gh.Issues.Create(ctx, repository.GetOwner().GetLogin(), repository.GetName(), issue)
			if err != nil {
				log.Printf("failed to create issue: %v", err)
			}

			continue
		}

		err = s.store(data)
		if err != nil {
			log.Printf("failed to store plugin %s: %v", data.Name, err)
		}
	}

	return nil
}

func (s *Scrapper) isSkipped(ctx context.Context, repository *github.Repository) bool {
	if _, ok := s.blacklist[repository.GetFullName()]; ok {
		return true
	}

	if s.hasIssue(ctx, repository) {
		log.Printf("[%s] the issue is still opened.", repository.GetFullName())
		return true
	}

	return false
}

func (s *Scrapper) hasIssue(ctx context.Context, repository *github.Repository) bool {
	user, _, err := s.gh.Users.Get(ctx, "")
	if err != nil {
		log.Println(err)
		return false
	}

	opts := &github.IssueListByRepoOptions{
		State:   "open",
		Creator: user.GetLogin(),
	}

	issues, _, err := s.gh.Issues.ListByRepo(ctx, repository.GetOwner().GetLogin(), repository.GetName(), opts)
	if err != nil {
		log.Println(err)
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

	// Gets module information

	// FIXME optional module file?
	mod, err := s.getModuleInfo(ctx, repository, latestVersion)
	if err != nil {
		return nil, err
	}

	moduleName := mod.Module.Mod.Path

	// Checks consistency

	if !strings.HasPrefix(manifest.Import, moduleName) {
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
		err = yaegiCheck(gop, manifest)
		if err != nil {
			return nil, fmt.Errorf("failed to run with Yaegi: %w", err)
		}
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
		Snippet:       manifest.TestData,
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
		log.Println("[INFO]", err)

		err = s.pg.Create(*data)
		if err != nil {
			return err
		}

		log.Println("Stored:", data.Name)
		return nil
	}

	data.ID = prev.ID
	err = s.pg.Update(*data)
	if err != nil {
		return err
	}

	log.Println("Stored:", data.Name)
	return nil
}

func yaegiCheck(goPath string, manifest Manifest) error {
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})
	ctx := context.Background()
	middlewareName := "test"

	i := interp.New(interp.Options{GoPath: goPath})
	i.Use(stdlib.Symbols)

	_, err := i.Eval(fmt.Sprintf(`import "%s"`, manifest.Import))
	if err != nil {
		return fmt.Errorf("plugin: failed to import plugin code: %w", err)
	}

	basePkg := manifest.BasePkg
	if basePkg == "" {
		basePkg = path.Base(manifest.Import)
		basePkg = strings.ReplaceAll(basePkg, "-", "_")
	}

	vConfig, err := i.Eval(basePkg + `.CreateConfig()`)
	if err != nil {
		return fmt.Errorf("plugin: failed to eval CreateConfig: %w", err)
	}

	err = mapstructure.Decode(manifest.TestData, vConfig.Interface())
	if err != nil {
		return fmt.Errorf("plugin: failed to decode configuration: %w", err)
	}

	fnNew, err := i.Eval(basePkg + `.New`)
	if err != nil {
		return fmt.Errorf("plugin: failed to eval New: %w", err)
	}

	args := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(next), vConfig, reflect.ValueOf(middlewareName)}
	results := fnNew.Call(args)

	if len(results) > 1 && results[1].Interface() != nil {
		return fmt.Errorf("plugin: failed to create a new plugin instance: %w", results[1].Interface().(error))
	}

	_, ok := results[0].Interface().(http.Handler)
	if !ok {
		return fmt.Errorf("plugin: invalid handler type: %T", results[0].Interface())
	}

	return nil
}
