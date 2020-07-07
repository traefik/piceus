package core

import (
	"context"

	"github.com/google/go-github/v32/github"
	"golang.org/x/mod/module"
)

// Sources gets code sources.
type Sources interface {
	Get(ctx context.Context, repository *github.Repository, gop string, mod module.Version) error
}

// Manifest The plugin manifest.
type Manifest struct {
	DisplayName   string                 `yaml:"displayName"`
	Type          string                 `yaml:"type"`
	Import        string                 `yaml:"import"`
	BasePkg       string                 `yaml:"basePkg"`
	Compatibility string                 `yaml:"compatibility"`
	Summary       string                 `yaml:"summary"`
	TestData      map[string]interface{} `yaml:"testData"`
}
