package sources

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v57/github"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/module"
	"golang.org/x/oauth2"
)

func TestGHSources_Get(t *testing.T) {
	ctx := context.Background()

	client := newGitHubClient(ctx, "")
	sources := GitHub{Client: client}

	// require because the process modifies chdir.
	wd, err := os.Getwd()
	require.NoError(t, err)

	orig := filepath.Join(wd, "test")

	t.Cleanup(func() {
		_ = os.Chdir(wd)
		_ = os.RemoveAll(orig)
	})

	repo := &github.Repository{
		Name: github.String("grignotin"),
		Owner: &github.User{
			Login: github.String("ldez"),
		},
	}

	err = sources.Get(ctx, repo, "./test", module.Version{Path: "github.com/ldez/grignotin", Version: "v0.1.0"})
	require.NoError(t, err)
}

func newGitHubClient(ctx context.Context, token string) *github.Client {
	if token == "" {
		return github.NewClient(nil)
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	return github.NewClient(oauth2.NewClient(ctx, ts))
}
