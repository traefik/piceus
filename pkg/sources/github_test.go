package sources

import (
	"context"
	"os"
	"testing"

	"github.com/google/go-github/v45/github"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/module"
	"golang.org/x/oauth2"
)

func TestGHSources_Get(t *testing.T) {
	ctx := context.Background()

	client := newGitHubClient(ctx, "")
	sources := GitHub{Client: client}

	t.Cleanup(func() {
		_ = os.RemoveAll("./test")
	})

	repo := &github.Repository{
		Name: github.String("grignotin"),
		Owner: &github.User{
			Login: github.String("ldez"),
		},
	}

	_, err := sources.Get(ctx, repo, "./test", module.Version{Path: "github.com/ldez/grignotin", Version: "v0.1.0"})
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
