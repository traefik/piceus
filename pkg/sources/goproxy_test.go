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

	t.Cleanup(func() {
		_ = os.RemoveAll("./test")
	})

	_, err := sources.Get(context.Background(), nil, "./test", module.Version{Path: "github.com/ldez/grignotin", Version: "v0.1.0"})
	require.NoError(t, err)
}
