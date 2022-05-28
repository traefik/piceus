package core

import (
	"context"

	"github.com/google/go-github/v45/github"
	"golang.org/x/mod/module"
)

// Sources gets code sources.
type Sources interface {
	Get(ctx context.Context, repository *github.Repository, gop string, mod module.Version) error
}

// Manifest The plugin manifest.
type Manifest struct {
	DisplayName   string                 `json:"displayName,omitempty" toml:"displayName,omitempty" yaml:"displayName,omitempty"`
	Type          string                 `json:"type,omitempty" toml:"type,omitempty" yaml:"type,omitempty"`
	Import        string                 `json:"import,omitempty" toml:"import,omitempty" yaml:"import,omitempty"`
	BasePkg       string                 `json:"basePkg,omitempty" toml:"basePkg,omitempty" yaml:"basePkg,omitempty"`
	Compatibility string                 `json:"compatibility,omitempty" toml:"compatibility,omitempty" yaml:"compatibility,omitempty"`
	Summary       string                 `json:"summary,omitempty" toml:"summary,omitempty" yaml:"summary,omitempty"`
	IconPath      string                 `json:"iconPath,omitempty" toml:"iconPath,omitempty" yaml:"iconPath,omitempty"`
	BannerPath    string                 `json:"bannerPath,omitempty" toml:"bannerPath,omitempty" yaml:"bannerPath,omitempty"`
	TestData      map[string]interface{} `json:"testData,omitempty" toml:"testData,omitempty" yaml:"testData,omitempty"`
}
