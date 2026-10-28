package sources

import (
	"context"
	"errors"
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

func TestGitHub_ref(t *testing.T) {
	mod := module.Version{Path: "github.com/o/r", Version: "v1.0.0"}

	pinned := func(commit string, err error) func(context.Context, string, string) (string, error) {
		return func(context.Context, string, string) (string, error) { return commit, err }
	}

	testCases := []struct {
		desc    string
		pinned  func(context.Context, string, string) (string, error)
		ref     string
		wantErr bool
	}{
		{desc: "no proxy", ref: "v1.0.0"},
		{desc: "pinned", pinned: pinned("abc", nil), ref: "abc"},
		{desc: "not pinned", pinned: pinned("", nil), ref: "v1.0.0"},
		{desc: "proxy down", pinned: pinned("", errors.New("502")), wantErr: true},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			s := GitHub{PinnedCommit: test.pinned}

			ref, err := s.ref(t.Context(), mod)
			if test.wantErr {
				require.ErrorIs(t, err, ErrPinnedCommit)
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.ref, ref)
		})
	}
}
