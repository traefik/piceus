package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/piceus/pkg/sandbox"
	"github.com/traefik/yaegi/stdlib"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

const (
	// evaluatorTestEnv makes the test binary act as the evaluator process.
	evaluatorTestEnv = "PICEUS_TEST_EVALUATOR"
	// noSandboxTestEnv disables the sandbox in the evaluator process, when the kernel cannot enforce it.
	noSandboxTestEnv = "PICEUS_TEST_NO_SANDBOX"
	// fullSymbolsTestEnv gives the full Yaegi stdlib to the plugin: the symbol filter is disabled.
	fullSymbolsTestEnv = "PICEUS_TEST_FULL_SYMBOLS"
)

func TestMain(m *testing.M) {
	if os.Getenv(evaluatorTestEnv) != "" {
		os.Exit(runTestEvaluator())
	}

	os.Exit(m.Run())
}

func TestEvaluator_Evaluate(t *testing.T) {
	testCases := []struct {
		desc        string
		fixture     string
		timeout     time.Duration
		sandbox     bool
		fullSymbols bool
		expectError bool
		contains    []string
		notContains []string
		isFailure   bool
	}{
		{
			desc:    "valid plugin",
			fixture: "simple",
		},
		{
			desc:    "plugin output on stdout and stderr is not the result",
			fixture: "noisy",
		},
		{
			desc:        "unsafe import without useUnsafe",
			fixture:     "wrongunsafe",
			expectError: true,
		},
		{
			desc:        "net/http/cgi cannot start a shell",
			fixture:     "cgi",
			expectError: true,
			contains:    []string{"net/http/cgi"},
			notContains: []string{"pwned"},
		},
		{
			desc:        "os.StartProcess is not available",
			fixture:     "startprocess",
			expectError: true,
			contains:    []string{"StartProcess"},
			notContains: []string{"pwned"},
		},
		{
			desc:        "files outside of the work directory cannot be read",
			fixture:     "secrets",
			sandbox:     true,
			expectError: true,
			contains:    []string{"/environ: permission denied", "/etc/passwd: permission denied"},
			notContains: []string{"LEAK"},
		},
		{
			desc:        "a blocked plugin is killed at the timeout",
			fixture:     "hang",
			timeout:     2 * time.Second,
			expectError: true,
			contains:    []string{"the plugin evaluation timed out after 2s"},
		},
		{
			desc:        "a crash is an evaluator failure",
			fixture:     "crash",
			expectError: true,
			isFailure:   true,
		},
		{
			desc:        "os.NewFile is not available",
			fixture:     "fakeresult",
			expectError: true,
			contains:    []string{"NewFile"},
		},
		{
			// The full symbols give os.NewFile to the plugin: only the exit status stops the fake result.
			desc:        "a valid result written by the plugin before a crash is an evaluator failure",
			fixture:     "fakeresult",
			fullSymbols: true,
			expectError: true,
			isFailure:   true,
		},
		{
			desc:        "an error larger than the result limit is truncated",
			fixture:     "bigerror",
			timeout:     5 * time.Second,
			expectError: true,
			contains:    []string{"xxx" + issueDetailTruncation},
			notContains: []string{"timed out"},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			if test.sandbox {
				requireSandbox(t)
			}

			manifest, goPath := loadFixture(t, test.fixture)

			evaluator := newTestEvaluator(t)
			if test.timeout > 0 {
				evaluator.timeout = test.timeout
			}

			if test.fullSymbols {
				evaluator.env = append(evaluator.env, fullSymbolsTestEnv+"=1")
			}

			start := time.Now()
			err := evaluator.Evaluate(t.Context(), evaluationRequest{
				Manifest: manifest,
				WorkDir:  goPath,
				GoPath:   goPath,
			})

			if test.timeout > 0 {
				assert.Less(t, time.Since(start), test.timeout+evaluatorWaitDelay+time.Second)
			}

			if !test.expectError {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Equal(t, test.isFailure, errors.Is(err, errEvaluatorFailure), err.Error())

			for _, s := range test.contains {
				assert.Contains(t, err.Error(), s)
			}

			for _, s := range test.notContains {
				assert.NotContains(t, err.Error(), s)
			}
		})
	}
}

// TestEvaluator_Evaluate_pentestV01 runs the pentest V01 proof of concept without the Yaegi symbol filter.
// The sandbox alone must stop the shell.
func TestEvaluator_Evaluate_pentestV01(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is not available")
	}

	testCases := []struct {
		desc        string
		sandbox     bool
		contains    []string
		notContains []string
	}{
		{
			desc:    "the sandbox blocks the shell",
			sandbox: true,
			// The plugin imported net/http/cgi and New ran: only the sandbox stopped the shell.
			contains:    []string{"failed to create a new plugin instance: output: "},
			notContains: []string{"pwned"},
		},
		{
			// Control case: it proves that the fixture really starts a shell.
			desc:     "without the sandbox, the shell runs",
			sandbox:  false,
			contains: []string{"pwned"},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			if test.sandbox {
				requireSandbox(t)
			}

			manifest, goPath := loadFixture(t, "cgi")

			evaluator := newTestEvaluator(t)
			evaluator.env = append(evaluator.env, fullSymbolsTestEnv+"=1")
			if !test.sandbox {
				evaluator.env = append(evaluator.env, noSandboxTestEnv+"=1")
			}

			err := evaluator.Evaluate(t.Context(), evaluationRequest{
				Manifest: manifest,
				WorkDir:  goPath,
				GoPath:   goPath,
			})
			require.Error(t, err)

			for _, s := range test.contains {
				assert.Contains(t, err.Error(), s)
			}

			for _, s := range test.notContains {
				assert.NotContains(t, err.Error(), s)
			}
		})
	}
}

func TestEvaluator_Evaluate_emptyEnvironment(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token-value")

	manifest, goPath := loadFixture(t, "secrets")

	err := newTestEvaluator(t).Evaluate(t.Context(), evaluationRequest{
		Manifest: manifest,
		WorkDir:  goPath,
		GoPath:   goPath,
	})
	require.Error(t, err)

	assert.NotContains(t, err.Error(), "secret-token-value")
	assert.NotContains(t, err.Error(), "GITHUB_TOKEN")
}

func TestEvaluator_Evaluate_startFailure(t *testing.T) {
	evaluator := &Evaluator{
		command: []string{filepath.Join(t.TempDir(), "missing")},
		env:     []string{},
		timeout: time.Second,
	}

	err := evaluator.Evaluate(t.Context(), evaluationRequest{WorkDir: t.TempDir()})
	require.Error(t, err)

	assert.ErrorIs(t, err, errEvaluatorFailure)
}

func TestNewEvaluator(t *testing.T) {
	testCases := []struct {
		desc        string
		opts        []EvaluatorOption
		expected    time.Duration
		expectError bool
	}{
		{
			desc:     "default timeout",
			expected: DefaultEvaluationTimeout,
		},
		{
			desc:     "custom timeout",
			opts:     []EvaluatorOption{WithEvaluationTimeout(5 * time.Minute)},
			expected: 5 * time.Minute,
		},
		{
			desc:        "zero timeout",
			opts:        []EvaluatorOption{WithEvaluationTimeout(0)},
			expectError: true,
		},
		{
			desc:        "negative timeout",
			opts:        []EvaluatorOption{WithEvaluationTimeout(-time.Second)},
			expectError: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			evaluator, err := NewEvaluator(func() error { return nil }, test.opts...)
			if test.expectError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expected, evaluator.timeout)
		})
	}
}

func TestNewEvaluator_sandboxNotAvailable(t *testing.T) {
	_, err := NewEvaluator(func() error { return errors.New("landlock is not available") })
	require.ErrorContains(t, err, "landlock is not available")
}

func Test_sanitizeIssueDetail(t *testing.T) {
	testCases := []struct {
		desc     string
		detail   string
		expected string
	}{
		{
			desc:     "short detail",
			detail:   "failed to eval `New` function",
			expected: "failed to eval 'New' function",
		},
		{
			desc:     "code block cannot be closed",
			detail:   "```\n# injected",
			expected: "'''\n# injected",
		},
		{
			desc:     "invalid UTF-8",
			detail:   "a\xffb",
			expected: "a?b",
		},
		{
			desc:     "long detail",
			detail:   strings.Repeat("a", maxIssueDetailLength+10),
			expected: strings.Repeat("a", maxIssueDetailLength) + issueDetailTruncation,
		},
		{
			desc:     "long detail is cut on a rune boundary",
			detail:   strings.Repeat("a", maxIssueDetailLength-1) + "é" + "b",
			expected: strings.Repeat("a", maxIssueDetailLength-1) + issueDetailTruncation,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.expected, sanitizeIssueDetail(test.detail))
		})
	}
}

func runTestEvaluator() int {
	restrict := sandbox.Apply
	if os.Getenv(noSandboxTestEnv) != "" {
		restrict = func(string) error { return nil }
	}

	symbols := evaluationSymbols()
	if os.Getenv(fullSymbolsTestEnv) != "" {
		symbols = stdlib.Symbols
	}

	result := os.NewFile(EvaluatorResultFD, "result")
	if err := runEvaluation(context.Background(), os.Stdin, result, restrict, symbols); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		return 3
	}

	return 0
}

// newTestEvaluator creates an Evaluator that starts the test binary as the evaluator process.
func newTestEvaluator(t *testing.T) *Evaluator {
	t.Helper()

	env := []string{evaluatorTestEnv + "=1"}
	if err := sandbox.Available(); err != nil {
		t.Logf("sandbox not available, the evaluator runs without it: %v", err)
		env = append(env, noSandboxTestEnv+"=1")
	}

	return &Evaluator{
		command: []string{os.Args[0]},
		env:     env,
		timeout: 30 * time.Second,
	}
}

func requireSandbox(t *testing.T) {
	t.Helper()

	if err := sandbox.Available(); err != nil {
		t.Skipf("sandbox not available: %v", err)
	}
}

// loadFixture reads the manifest of a fixture plugin, and copies its sources into a new GOPATH.
func loadFixture(t *testing.T, name string) (Manifest, string) {
	t.Helper()

	rootDir := filepath.Join("fixtures", name)

	manifestBytes, err := os.ReadFile(filepath.Join(rootDir, manifestFile))
	require.NoError(t, err)

	var manifest Manifest
	require.NoError(t, yaml.Unmarshal(manifestBytes, &manifest))

	content, err := os.ReadFile(filepath.Join(rootDir, "go.mod"))
	require.NoError(t, err)

	mod, err := modfile.Parse("go.mod", content, nil)
	require.NoError(t, err)

	goPath := t.TempDir()
	source := LocalSources{src: rootDir}
	err = source.Get(t.Context(), nil, goPath, module.Version{Path: mod.Module.Mod.Path}, "")
	require.NoError(t, err)

	return manifest, goPath
}
