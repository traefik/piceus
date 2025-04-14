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
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

func TestUnsafe(t *testing.T) {

	testCases := []struct {
		desc        string
		rootDir     string
		expectError bool
	}{
		{
			desc:        "Simple plugin should pass",
			rootDir:     filepath.Join("fixtures", "simple"),
			expectError: false,
		},
		{
			desc:        "Simple plugin with unsafe  without manifest.unsafe should fail",
			rootDir:     filepath.Join("fixtures", "wrongunsafe"),
			expectError: true,
		},

		{
			desc:        "Simple plugin with unsafe",
			rootDir:     filepath.Join("fixtures", "unsafe"),
			expectError: false,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			manifestBytes, err := os.ReadFile(filepath.Join(test.rootDir, manifestFile))
			require.NoError(t, err)

			var manifest Manifest
			err = yaml.Unmarshal(manifestBytes, &manifest)
			require.NoError(t, err)

			content, err := os.ReadFile(filepath.Join(test.rootDir, "go.mod"))
			require.NoError(t, err)

			mod, err := modfile.Parse("go.mod", content, nil)
			require.NoError(t, err)

			tmpdir := t.TempDir()
			source := LocalSources{src: test.rootDir}
			err = source.Get(nil, nil, tmpdir, module.Version{
				Path: mod.Module.Mod.Path,
			})

			s := Scrapper{}
			require.NoError(t, err)

			err = s.yaegiCheck(manifest, tmpdir, "")
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

func (s *LocalSources) Get(_ context.Context, _ *github.Repository, gop string, mod module.Version) error {
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

		new, err := os.Create(filepath.Join(dest, d.Name()))
		if err != nil {
			return fmt.Errorf("failed to create source file: %w", err)
		}

		orig, err := os.Open(filepath.Join(s.src, d.Name()))
		if err != nil {
			return fmt.Errorf("failed to open source file: %w", err)
		}

		_, err = io.Copy(new, orig)
		if err != nil {
			return fmt.Errorf("failed to copy source file: %w", err)
		}
	}

	return nil

}
