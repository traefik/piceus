package core

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"

	"github.com/google/go-github/v57/github"
)

const wasmFile = "plugin.wasm"

// maxZipSize is the maximum size of the release zip archive of a WASM plugin.
const maxZipSize = 256 << 20

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
		err = s.verifyZip(ctx, repository.GetOwner().GetLogin(), repository.GetName(), asset.GetID(), manifest)
		if err != nil {
			return fmt.Errorf("invalid zip archive content: %w", err)
		}
	}

	return nil
}

func (s *Scrapper) verifyZip(ctx context.Context, owner, repo string, assetID int64, manifest Manifest) error {
	asset, _, err := s.gh.Repositories.DownloadReleaseAsset(ctx, owner, repo, assetID, s.gh.Client())
	if err != nil {
		return fmt.Errorf("failed to download asset: %w", err)
	}

	defer func() { _ = asset.Close() }()

	body, err := io.ReadAll(io.LimitReader(asset, maxZipSize+1))
	if err != nil {
		return fmt.Errorf("failed to read asset body: %w", err)
	}

	if len(body) > maxZipSize {
		return fmt.Errorf("zip archive is too large (limit: %d bytes)", maxZipSize)
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

	// Only the presence of the files is checked: the manifest of the zip is not read, and the plugin binary is not run.
	return nil
}

func getWasmPath(manifest Manifest) (string, error) {
	wasmPath := manifest.WasmPath
	if wasmPath == "" {
		wasmPath = wasmFile
	}

	if !filepath.IsLocal(wasmPath) {
		return "", errors.New("wasmPath must be a local path")
	}

	return wasmPath, nil
}
