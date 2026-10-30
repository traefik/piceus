package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v57/github"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/module"
)

func TestUnsafe(t *testing.T) {
	testCases := []struct {
		desc        string
		fixture     string
		expectError bool
	}{
		{
			desc:        "Simple plugin should pass",
			fixture:     "simple",
			expectError: false,
		},
		{
			desc:        "Simple plugin with unsafe without manifest.unsafe should fail",
			fixture:     "wrongunsafe",
			expectError: true,
		},

		{
			desc:        "Simple plugin with unsafe",
			fixture:     "unsafe",
			expectError: false,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			manifest, goPath := loadFixture(t, test.fixture)

			s := Scrapper{evaluator: newTestEvaluator(t)}

			err := s.yaegiCheck(t.Context(), manifest, goPath, "")
			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

type LocalSources struct {
	src string
}

func (s *LocalSources) Get(_ context.Context, _ *github.Repository, gop string, mod module.Version, _ string) error {
	dest := filepath.Join(filepath.Join(gop, "src"), filepath.FromSlash(mod.Path))
	err := os.MkdirAll(dest, 0o750)
	if err != nil {
		return fmt.Errorf("failed to create sources directory: %w", err)
	}

	dir, err := os.ReadDir(s.src)
	if err != nil {
		return fmt.Errorf("failed to read sources directory: %w", err)
	}

	for _, d := range dir {
		if d.IsDir() {
			return errors.New("unexpected directory")
		}

		newFile, err := os.Create(filepath.Join(dest, d.Name()))
		if err != nil {
			return fmt.Errorf("failed to create source file: %w", err)
		}

		orig, err := os.Open(filepath.Join(s.src, d.Name()))
		if err != nil {
			return fmt.Errorf("failed to open source file: %w", err)
		}

		_, err = io.Copy(newFile, orig)
		if err != nil {
			return fmt.Errorf("failed to copy source file: %w", err)
		}
	}

	return nil
}

func TestCheckRepoName(t *testing.T) {
	testCases := []struct {
		desc        string
		repository  *github.Repository
		moduleName  string
		manifest    Manifest
		expectError bool
	}{
		{
			desc: "Fails if repo name is not prefix of module name",
			repository: &github.Repository{
				FullName: new("foo/bar"),
			},
			moduleName: "github.com/something/wrong",
			manifest: Manifest{
				Import: "github.com/foo/bar",
			},
			expectError: true,
		},
		{
			desc: "Fails if repo name is not prefix of manifest import",
			repository: &github.Repository{
				FullName: new("foo/bar"),
			},
			moduleName: "github.com/foo/bar",
			manifest: Manifest{
				Import: "github.com/something/wrong",
			},
			expectError: true,
		},
		{
			desc: "Succeeds",
			repository: &github.Repository{
				FullName: new("foo/bar"),
			},
			moduleName: "github.com/foo/bar",
			manifest: Manifest{
				Import: "github.com/foo/bar",
			},
			expectError: false,
		},
		{
			desc: "Succeeds if repo name contains uppercase characters, but module name and manifest import do not",
			repository: &github.Repository{
				FullName: new("FOO/bar"),
			},
			moduleName: "github.com/foo/bar",
			manifest: Manifest{
				Import: "github.com/foo/bar",
			},
			expectError: false,
		},
		{
			desc: "Succeeds with mixed case in both repo name and module name",
			repository: &github.Repository{
				FullName: new("Foo/Bar"),
			},
			moduleName: "github.com/Foo/Bar",
			manifest: Manifest{
				Import: "github.com/Foo/Bar",
			},
			expectError: false,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			err := checkRepoName(test.repository, test.moduleName, test.manifest)

			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
