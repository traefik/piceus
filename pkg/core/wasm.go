package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"

	"github.com/google/go-github/v45/github"
	"github.com/http-wasm/http-wasm-host-go/handler"
	wasm "github.com/http-wasm/http-wasm-host-go/handler/nethttp"
)

const wasmFile = "plugin.wasm"

func (s *Scrapper) verifyWASMPlugin(ctx context.Context, repository *github.Repository, latestVersion string, manifest Manifest) (string, []string, error) {
	pluginName := path.Join("github.com", repository.GetFullName())

	// skip already existing plugin
	prev, err := s.pg.GetByName(ctx, pluginName)
	if err == nil && prev != nil && prev.LatestVersion == latestVersion && prev.Stars == repository.GetStargazersCount() {
		return "", nil, nil
	}

	// Get versions
	versions, err := s.getVersions(ctx, repository, pluginName)
	if err != nil {
		return "", nil, err
	}

	err = s.verifyRelease(ctx, repository, manifest)
	if err != nil {
		return "", nil, fmt.Errorf("verify release assets failed: %w", err)
	}

	return pluginName, versions, nil
}

func (s *Scrapper) verifyRelease(ctx context.Context, repository *github.Repository, manifest Manifest) error {
	release, _, err := s.gh.Repositories.GetLatestRelease(ctx, repository.GetOwner().GetLogin(), repository.GetName())
	if err != nil {
		return fmt.Errorf("failed to get latest release: %w", err)
	}

	assets := map[*github.ReleaseAsset]struct{}{}
	for _, asset := range release.Assets {
		if filepath.Ext(asset.GetName()) == ".zip" {
			assets[asset] = struct{}{}
		}
	}

	if len(assets) > 1 {
		return fmt.Errorf("too many zip archive (%d)", len(assets))
	}

	if len(assets) == 0 {
		return errors.New("zip archive not found")
	}

	for asset := range assets {
		err = verifyZip(asset, manifest)
		if err != nil {
			return fmt.Errorf("invalid zip archive content: %w", err)
		}
	}

	return nil
}

func verifyZip(asset *github.ReleaseAsset, manifest Manifest) error {
	resp, err := http.Get(asset.GetBrowserDownloadURL())
	if err != nil {
		return fmt.Errorf("failed to download asset: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read asset body: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("failed to unzip archive: %w", err)
	}

	wasmPath, err := getWasmPath(manifest)
	if err != nil {
		return err
	}

	var foundManifest bool
	var wasmPluginFile *zip.File

	for _, file := range reader.File {
		switch file.Name {
		case wasmPath:
			wasmPluginFile = file
		case manifestFile:
			foundManifest = true
		}

		if foundManifest && wasmPluginFile != nil {
			break
		}
	}

	if wasmPluginFile == nil {
		return errors.New("failed to find " + wasmPath)
	}

	if !foundManifest {
		return errors.New("failed to find " + manifestFile)
	}

	switch manifest.Type {
	case typeMiddleware:
		err = checkWasmMiddleware(wasmPluginFile, manifest)
		if err != nil {
			return fmt.Errorf("failed to check wasm middleware: %w", err)
		}

	case typeProvider:
		// TODO add support?
		return nil

	default:
		return fmt.Errorf("unsupported type: %s", manifest.Type)
	}

	return nil
}

func checkWasmMiddleware(file *zip.File, manifest Manifest) error {
	readCloser, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open wasm file: %w", err)
	}

	pluginBytes, err := io.ReadAll(readCloser)
	if err != nil {
		return fmt.Errorf("failed to read wasm file: %w", err)
	}

	b, err := json.Marshal(manifest.TestData)
	if err != nil {
		return fmt.Errorf("failed to marshal test data: %w", err)
	}

	_, err = wasm.NewMiddleware(context.Background(), pluginBytes, handler.GuestConfig(b))
	if err != nil {
		return fmt.Errorf("failed to interpret plugin: %w", err)
	}

	return nil
}

func getWasmPath(manifest Manifest) (string, error) {
	wasmPath := manifest.WasmPath
	if wasmPath == "" {
		wasmPath = wasmFile
	}

	if !filepath.IsLocal(wasmPath) {
		return "", fmt.Errorf("wasmPath must be a local path")
	}

	return wasmPath, nil
}
