package sources

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v32/github"
	"golang.org/x/mod/module"
)

// GitHub gets sources from GitHub.
type GitHub struct {
	Client *github.Client
}

// Get gets sources.
func (s *GitHub) Get(ctx context.Context, repository *github.Repository, gop string, mod module.Version) error {
	// Creates temp archive storage

	rootArchive, err := ioutil.TempDir("", "plaen-archives")
	if err != nil {
		return fmt.Errorf("failed to create temp archive storage: %w", err)
	}

	defer func() { _ = os.RemoveAll(rootArchive) }()

	// Gets code (archive)

	archivePath, err := s.getArchive(ctx, repository, mod.Version, rootArchive)

	defer func() {
		if archivePath != "" {
			_ = os.RemoveAll(archivePath)
		}
	}()

	if err != nil {
		return fmt.Errorf("failed to get code (archive): %w", err)
	}

	// Gets code (sources)

	dest := filepath.Join(filepath.Join(gop, "src"), filepath.FromSlash(mod.Path))
	err = os.MkdirAll(dest, 0750)
	if err != nil {
		return fmt.Errorf("failed to create sources directory: %w", err)
	}

	err = unzip(archivePath, dest)
	if err != nil {
		return fmt.Errorf("failed to unzip archive: %w", err)
	}

	return nil
}

func (s *GitHub) getArchive(ctx context.Context, repository *github.Repository, version string, rootArchive string) (string, error) {
	opts := &github.RepositoryContentGetOptions{Ref: version}

	link, _, err := s.Client.Repositories.GetArchiveLink(ctx, repository.GetOwner().GetLogin(), repository.GetName(), github.Zipball, opts, true)
	if err != nil {
		return "", fmt.Errorf("failed to get archive link: %w", err)
	}

	request, err := http.NewRequest(http.MethodGet, link.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// TODO remove hardcoded github.com?
	filename := filepath.Join(rootArchive, "github.com", filepath.FromSlash(repository.GetFullName()), version+".zip")

	err = os.MkdirAll(filepath.Dir(filename), 0750)
	if err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	arch, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("failed to create archive file: %w", err)
	}
	defer func() { _ = arch.Close() }()

	_, err = s.Client.Do(ctx, request, arch)
	if err != nil {
		return "", fmt.Errorf("failed to get archive: %w", err)
	}

	return filename, nil
}

func unzip(zipPath, dest string) error {
	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}

	defer func() { _ = archive.Close() }()

	for _, f := range archive.File {
		err = unzipFile(f, dest)
		if err != nil {
			return err
		}
	}

	return nil
}

func unzipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}

	defer func() { _ = rc.Close() }()

	pathParts := strings.SplitN(f.Name, string(os.PathSeparator), 2)
	p := filepath.Join(dest, pathParts[1])

	if f.FileInfo().IsDir() {
		return os.MkdirAll(p, f.Mode())
	}

	err = os.MkdirAll(filepath.Dir(p), 0750)
	if err != nil {
		return err
	}

	elt, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}

	defer func() { _ = elt.Close() }()

	_, err = io.Copy(elt, rc)
	if err != nil {
		return err
	}

	return nil
}