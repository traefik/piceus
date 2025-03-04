package sources

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ldez/grignotin/goproxy"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/module"
)

func TestGPSources_Get(t *testing.T) {
	gpClient := goproxy.NewClient("")
	sources := GoProxy{Client: gpClient}

	wd, err := os.Getwd()
	require.NoError(t, err)

	// require because the process modifies chdir.
	orig := filepath.Join(wd, "test")

	t.Cleanup(func() {
		_ = os.Chdir(wd)
		_ = os.RemoveAll(orig)
	})

	err = sources.Get(context.Background(), nil, "./test", module.Version{Path: "github.com/ldez/grignotin", Version: "v0.1.0"})
	require.NoError(t, err)
}
