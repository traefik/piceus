package core

import (
	"maps"
	"slices"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// evaluationSymbols returns the Yaegi stdlib symbols without the packages and symbols that start or control processes.
// The evaluator sandbox also blocks these operations: this is defense in depth.
func evaluationSymbols() interp.Exports {
	// net/http/cgi starts programs.
	deniedPackages := []string{"net/http/cgi/cgi"}

	deniedSymbols := map[string][]string{
		"os/os": {"StartProcess", "FindProcess"},
	}

	symbols := make(interp.Exports, len(stdlib.Symbols))

	for pkg, values := range stdlib.Symbols {
		if slices.Contains(deniedPackages, pkg) {
			continue
		}

		values = maps.Clone(values)
		for _, name := range deniedSymbols[pkg] {
			delete(values, name)
		}

		symbols[pkg] = values
	}

	return symbols
}
