package sources

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v57/github"
	"golang.org/x/mod/module"
)

// ErrPinnedCommit the commit pinned by the Go module proxy could not be read: it is not a plugin problem.
var ErrPinnedCommit = errors.New("failed to get the commit pinned by the Go module proxy")

// GitHub gets sources from GitHub.
type GitHub struct {
	Client *github.Client
	// PinnedCommit returns the commit the Go module proxy pinned at publication, empty when it pinned none.
	// plugin-service serves that commit once the tag moved: the sources are read from it, not from the tag.
	PinnedCommit func(ctx context.Context, module, version string) (string, error)
}

// Get gets sources.
func (s *GitHub) Get(ctx context.Context, repository *github.Repository, gop string, mod module.Version) error {
	// Creates temp archive storage

	rootArchive, err := os.MkdirTemp("", "traefik-plugin-archives")
	if err != nil {
		return fmt.Errorf("failed to create temp archive storage: %w", err)
	}

	defer func() { _ = os.RemoveAll(rootArchive) }()

	// Gets code (archive)

	ref, err := s.ref(ctx, mod)
	if err != nil {
		return err
	}

	archivePath, err := s.getArchive(ctx, repository, mod.Version, ref, rootArchive)

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
	err = os.MkdirAll(dest, 0o750)
	if err != nil {
		return fmt.Errorf("failed to create sources directory: %w", err)
	}

	err = unzip(archivePath, dest)
	if err != nil {
		return fmt.Errorf("failed to unzip archive: %w", err)
	}

	// Change current dir to have testData path working.
	if err = os.Chdir(dest); err != nil {
		return fmt.Errorf("could not chdir to %q: %w", dest, err)
	}

	return nil
}

// ref returns the ref to read the sources of mod from: the pinned commit, or the tag when the proxy pinned none.
func (s *GitHub) ref(ctx context.Context, mod module.Version) (string, error) {
	if s.PinnedCommit == nil {
		return mod.Version, nil
	}

	pinned, err := s.PinnedCommit(ctx, mod.Path, mod.Version)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrPinnedCommit, err)
	}

	if pinned == "" {
		return mod.Version, nil
	}

	return pinned, nil
}

func (s *GitHub) getArchive(ctx context.Context, repository *github.Repository, version, ref, rootArchive string) (string, error) {
	opts := &github.RepositoryContentGetOptions{Ref: ref}

	link, _, err := s.Client.Repositories.GetArchiveLink(ctx, repository.GetOwner().GetLogin(), repository.GetName(), github.Zipball, opts, 3)
	if err != nil {
		return "", fmt.Errorf("failed to get archive link: %w", err)
	}

	request, err := http.NewRequest(http.MethodGet, link.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// TODO remove hardcoded github.com?
	filename := filepath.Join(rootArchive, "github.com", filepath.FromSlash(repository.GetFullName()), version+".zip")

	err = os.MkdirAll(filepath.Dir(filename), 0o750)
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

	err = os.MkdirAll(filepath.Dir(p), 0o750)
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
