package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/yaegi/stdlib"
)

func Test_evaluationSymbols(t *testing.T) {
	symbols := evaluationSymbols()

	assert.NotContains(t, symbols, "net/http/cgi/cgi")

	require.Contains(t, symbols, "os/os")
	assert.NotContains(t, symbols["os/os"], "StartProcess")
	assert.NotContains(t, symbols["os/os"], "FindProcess")
	assert.NotContains(t, symbols["os/os"], "NewFile")
	assert.Contains(t, symbols["os/os"], "ReadFile")

	// The original symbols must not change.
	assert.Contains(t, stdlib.Symbols, "net/http/cgi/cgi")
	assert.Contains(t, stdlib.Symbols["os/os"], "StartProcess")
}
