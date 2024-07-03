package metago

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	testCases := []struct {
		desc     string
		expected string
	}{
		{
			desc:     "github.com/stretchr/testify",
			expected: "github.com/stretchr/testify",
		},
		{
			desc:     "k8s.io/api",
			expected: "github.com/kubernetes/api",
		},
		{
			desc:     "go.elastic.co/apm",
			expected: "github.com/elastic/apm-agent-go",
		},
		{
			desc:     "gopkg.in/DataDog/dd-trace-go.v1",
			expected: "github.com/DataDog/dd-trace-go",
		},
		{
			desc:     "mvdan.cc/xurls/v2",
			expected: "github.com/mvdan/xurls",
		},
		{
			desc:     "gotest.tools",
			expected: "github.com/gotestyourself/gotest.tools",
		},
		{
			desc:     "gocloud.dev",
			expected: "github.com/google/go-cloud",
		},
		{
			desc:     "google.golang.org/appengine",
			expected: "github.com/golang/appengine",
		},
		{
			desc:     "golang.org/x/crypto",
			expected: "github.com/golang/crypto",
		},
		{
			desc:     "google.golang.org/grpc",
			expected: "github.com/grpc/grpc-go",
		},
		{
			desc:     "launchpad.net/gocheck",
			expected: "launchpad.net/gocheck",
		},
		{
			desc:     "code.gitea.io/sdk/gitea",
			expected: "gitea.com/gitea/go-sdk",
		},
		{
			desc:     "code.gitea.io/gitea",
			expected: "github.com/go-gitea/gitea",
		},
		{
			desc:     "gitlab.com/golang-commonmark/html",
			expected: "gitlab.com/golang-commonmark/html",
		},
		{
			desc:     "bitbucket.org/dtolpin/wigp",
			expected: "bitbucket.org/dtolpin/wigp",
		},
		{
			desc:     "bitbucket.org/StephaneBunel/xxhash-go",
			expected: "bitbucket.org/StephaneBunel/xxhash-go",
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			meta, err := Get(test.desc)
			require.NoError(t, err)

			t.Log(meta.GoImport)
			t.Log(meta.GoSource)
			assert.Equal(t, test.expected, EffectivePkgSource(meta))
		})
	}
}
