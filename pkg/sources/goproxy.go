package sources

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/go-github/v32/github"
	"github.com/ldez/grignotin/goproxy"
	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

// GoProxy gets sources from a GoProxy.
type GoProxy struct {
	Client *goproxy.Client
}

// Get gets sources.
func (s *GoProxy) Get(_ context.Context, _ *github.Repository, gop string, mod module.Version) error {
	// Creates temp archive storage

	rootArchive, err := ioutil.TempDir("", "plaen-archives")
	if err != nil {
		return fmt.Errorf("failed to create temp archive storage: %w", err)
	}

	defer func() { _ = os.RemoveAll(rootArchive) }()

	// Gets code (archive)

	archivePath, err := s.getArchive(mod, rootArchive)

	defer func() {
		if archivePath != "" {
			_ = os.RemoveAll(archivePath)
		}
	}()

	if err != nil {
		return fmt.Errorf("failed to get archive: %w", err)
	}

	// Gets code (sources)

	dest := filepath.Join(filepath.Join(gop, "src"), filepath.FromSlash(mod.Path))
	err = os.MkdirAll(dest, 0750)
	if err != nil {
		return fmt.Errorf("failed to create sources directory: %w", err)
	}

	return zip.Unzip(dest, mod, archivePath)
}

func (s *GoProxy) getArchive(mod module.Version, rootArchive string) (string, error) {
	reader, err := s.Client.DownloadSources(mod.Path, mod.Version)
	if err != nil {
		return "", err
	}

	defer func() { _ = reader.Close() }()

	archivePath := filepath.Join(rootArchive, filepath.FromSlash(mod.Path), mod.Version+".zip")
	err = os.MkdirAll(filepath.Dir(archivePath), 0750)
	if err != nil {
		return "", fmt.Errorf("failed to create sources directory: %w", err)
	}

	arch, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = arch.Close() }()

	_, err = io.Copy(arch, reader)
	if err != nil {
		return "", err
	}

	return archivePath, nil
}
