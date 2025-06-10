package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"reflect"
	"strings"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/mitchellh/mapstructure"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

func (s *Scrapper) verifyYaegiPlugin(ctx context.Context, repository *github.Repository, latestVersion string, manifest Manifest) (string, []string, error) {
	// Gets module information
	mod, err := s.getModuleInfo(ctx, repository, latestVersion)
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

	// Get versions
	versions, err := s.getVersions(ctx, repository, pluginName)
	if err != nil {
		return "", nil, err
	}

	// Creates temp GOPATH
	var gop string
	gop, err = os.MkdirTemp("", "traefik-plugin-gop")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp GOPATH: %w", err)
	}

	defer func() { _ = os.RemoveAll(gop) }()

	// Get sources
	err = s.sources.Get(ctx, repository, gop, module.Version{Path: pluginName, Version: latestVersion})
	if err != nil {
		return "", nil, fmt.Errorf("failed to get sources: %w", err)
	}

	// Check Yaegi interface
	err = s.yaegiCheck(manifest, gop, pluginName)
	if err != nil {
		return "", nil, fmt.Errorf("failed to run the plugin with Yaegi: %w", err)
	}

	return pluginName, versions, nil
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
		if manifest.UseUnsafe {
			// Skip unsafe test
			return nil
		}
		_, skip := s.skipNewCall[moduleName]
		return yaegiMiddlewareCheck(goPath, manifest, skip)

	case typeProvider:
		// TODO yaegi check for provider
		return nil

	default:
		return fmt.Errorf("unsupported type: %s", manifest.Type)
	}
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

func yaegiMiddlewareCheck(goPath string, manifest Manifest, skipNew bool) error {
	middlewareName := "test"

	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})

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
		return callNew(ctx, next, vConfig, middlewareName, fnNew)
	}

	return nil
}

func callNew(ctx context.Context, next http.HandlerFunc, vConfig reflect.Value, middlewareName string, fnNew reflect.Value) error {
	errCh := make(chan error)

	go func() {
		args := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(next), vConfig, reflect.ValueOf(middlewareName)}
		results, err := safeFnCall(fnNew, args)
		if err != nil {
			errCh <- fmt.Errorf("the function `New` of %s produce a panic: %w", middlewareName, err)
			return
		}

		if len(results) > 1 && results[1].Interface() != nil {
			errCh <- fmt.Errorf("failed to create a new plugin instance: %w", results[1].Interface().(error))
		}

		_, ok := results[0].Interface().(http.Handler)
		if !ok {
			errCh <- fmt.Errorf("invalid handler type: %T", results[0].Interface())
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("the function `New` has failed: %w", ctx.Err())
	}
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

func safeFnCall(fn reflect.Value, args []reflect.Value) (result []reflect.Value, errCall error) {
	defer func() {
		if err := recover(); err != nil {
			errCall = fmt.Errorf("panic during the call of the function: %v", err)
		}
	}()

	result = fn.Call(args)

	return
}
