package sources

import (
	"context"
	"os"
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

	// The tested function modifies the working directory with Chdir:
	// t.Chdir restores the initial working directory at the end of the test.
	wd, err := os.Getwd()
	require.NoError(t, err)

	t.Chdir(wd)

	repo := &github.Repository{
		Name: new("grignotin"),
		Owner: &github.User{
			Login: new("ldez"),
		},
	}

	err = sources.Get(ctx, repo, t.TempDir(), module.Version{Path: "github.com/ldez/grignotin", Version: "v0.1.0"})
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
