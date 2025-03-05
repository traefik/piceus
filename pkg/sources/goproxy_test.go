package sources

import (
	"context"
	"os"
	"testing"

	"github.com/ldez/grignotin/goproxy"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/module"
)

func TestGPSources_Get(t *testing.T) {
	gpClient := goproxy.NewClient("")
	sources := GoProxy{Client: gpClient}

	// Require because the tested function modifies the working directory with Chdir.
	wd, err := os.Getwd()
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	err = sources.Get(context.Background(), nil, t.TempDir(), module.Version{Path: "github.com/ldez/grignotin", Version: "v0.1.0"})
	require.NoError(t, err)
}
