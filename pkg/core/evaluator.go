package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/traefik/yaegi/interp"
)

// EvaluatorResultFD is the file descriptor on which the evaluator process writes its result.
// The plugin code can write to stdout and stderr, thus these streams do not carry the result.
const EvaluatorResultFD = 3

// DefaultEvaluationTimeout is the default maximum duration of one plugin evaluation.
// It is longer than the inner timeouts of the checks (Yaegi: 10s, WASM: 60s).
const DefaultEvaluationTimeout = 90 * time.Second

const (
	evaluatorWaitDelay    = 2 * time.Second
	maxEvaluationRequest  = 1 << 20
	maxEvaluationResult   = 64 << 10
	maxEvaluatorStderr    = 4 << 10
	maxIssueDetailLength  = 1024
	issueDetailTruncation = "... (truncated)"
)

// errEvaluatorFailure is returned when the evaluator process does not give a result.
// It is not a plugin problem: no issue is created.
var errEvaluatorFailure = errors.New("plugin evaluator failure")

// evaluationRequest is sent by Piceus to the evaluator process.
type evaluationRequest struct {
	Manifest Manifest `json:"manifest"`
	// WorkDir is the only directory that the evaluator can write to.
	WorkDir string `json:"workDir"`
	// GoPath contains the plugin sources (Yaegi plugins).
	GoPath string `json:"goPath,omitempty"`
	// WasmFile is the plugin binary (WASM plugins).
	WasmFile string `json:"wasmFile,omitempty"`
	SkipNew  bool   `json:"skipNew,omitempty"`
}

// evaluationResult is sent by the evaluator process to Piceus.
type evaluationResult struct {
	Error string `json:"error,omitempty"`
}

// Evaluator runs the plugin checks in a separate and sandboxed process.
// The process has an empty environment, and it is killed with all its children at the end of the evaluation.
type Evaluator struct {
	command []string
	env     []string
	timeout time.Duration
}

// EvaluatorOption configures an Evaluator.
type EvaluatorOption func(*Evaluator)

// WithEvaluationTimeout sets the maximum duration of one plugin evaluation.
// At the timeout, the evaluator process and its children are killed.
func WithEvaluationTimeout(timeout time.Duration) EvaluatorOption {
	return func(e *Evaluator) {
		e.timeout = timeout
	}
}

// NewEvaluator creates an Evaluator that runs the "evaluate" command of the current executable.
// sandboxAvailable must return an error if the sandbox cannot be enforced.
func NewEvaluator(sandboxAvailable func() error, opts ...EvaluatorOption) (*Evaluator, error) {
	if err := sandboxAvailable(); err != nil {
		return nil, fmt.Errorf("checking plugin sandbox: %w", err)
	}

	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("getting executable path: %w", err)
	}

	e := &Evaluator{
		command: []string{self, "evaluate"},
		env:     []string{},
		timeout: DefaultEvaluationTimeout,
	}

	for _, opt := range opts {
		opt(e)
	}

	// A zero or negative timeout stops every evaluation at once: every plugin would get an issue.
	if e.timeout <= 0 {
		return nil, fmt.Errorf("invalid evaluation timeout: %s", e.timeout)
	}

	return e, nil
}

// Evaluate runs the plugin checks described by req in the evaluator process.
func (e *Evaluator) Evaluate(ctx context.Context, req evaluationRequest) error {
	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("%w: marshaling request: %w", errEvaluatorFailure, err)
	}

	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	resultReader, resultWriter, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("%w: creating result pipe: %w", errEvaluatorFailure, err)
	}
	defer func() { _ = resultReader.Close() }()

	stderr := &limitedBuffer{max: maxEvaluatorStderr}

	cmd := exec.CommandContext(ctx, e.command[0], e.command[1:]...) //nolint:gosec // The command is the Piceus executable.
	// A non-nil empty slice gives an empty environment: the child must not see the secrets of Piceus.
	cmd.Env = e.env
	cmd.Dir = req.WorkDir
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stderr = stderr
	cmd.ExtraFiles = []*os.File{resultWriter}
	cmd.SysProcAttr = evaluatorSysProcAttr()
	cmd.Cancel = func() error { return killEvaluator(cmd.Process) }
	cmd.WaitDelay = evaluatorWaitDelay

	err = cmd.Start()
	_ = resultWriter.Close()
	if err != nil {
		return fmt.Errorf("%w: starting evaluator: %w", errEvaluatorFailure, err)
	}

	output, readErr := io.ReadAll(io.LimitReader(resultReader, maxEvaluationResult))
	waitErr := cmd.Wait()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("the plugin evaluation timed out after %s", e.timeout)
	}

	// The stderr of the evaluator helps to debug, and this error never goes into an issue.
	if readErr != nil {
		return fmt.Errorf("%w: reading result: %w (stderr: %q)", errEvaluatorFailure, errors.Join(readErr, waitErr), stderr)
	}

	var result evaluationResult
	if err = json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("%w: decoding result: %w (stderr: %q)", errEvaluatorFailure, errors.Join(err, waitErr), stderr)
	}

	if result.Error != "" {
		return errors.New(sanitizeIssueDetail(result.Error))
	}

	return nil
}

// RunEvaluation is the entry point of the evaluator process.
// It reads the request from in, calls restrict before any plugin code runs, and writes the result to out.
// It returns an error only when the evaluation cannot run: a plugin problem is written to out.
func RunEvaluation(ctx context.Context, in io.Reader, out io.Writer, restrict func(workDir string) error) error {
	return runEvaluation(ctx, in, out, restrict, evaluationSymbols())
}

// runEvaluation is RunEvaluation with the Yaegi symbols given to the plugin.
// Only the tests use other symbols than evaluationSymbols.
func runEvaluation(ctx context.Context, in io.Reader, out io.Writer, restrict func(workDir string) error, symbols interp.Exports) error {
	var req evaluationRequest
	if err := json.NewDecoder(io.LimitReader(in, maxEvaluationRequest)).Decode(&req); err != nil {
		return fmt.Errorf("decoding request: %w", err)
	}

	if req.WorkDir == "" {
		return errors.New("missing work directory")
	}

	if err := restrict(req.WorkDir); err != nil {
		return fmt.Errorf("restricting process: %w", err)
	}

	var result evaluationResult
	if err := evaluate(ctx, req, symbols); err != nil {
		result.Error = err.Error()
	}

	return json.NewEncoder(out).Encode(result)
}

func evaluate(ctx context.Context, req evaluationRequest, symbols interp.Exports) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic during the evaluation: %v", rec)
		}
	}()

	if req.Manifest.Runtime == wasmRuntime {
		pluginBytes, err := os.ReadFile(req.WasmFile)
		if err != nil {
			return fmt.Errorf("reading wasm file: %w", err)
		}

		return runWithTimeout(wasmCheckTimeout, func() error {
			return checkWasmMiddleware(ctx, pluginBytes, req.Manifest)
		})
	}

	return yaegiMiddlewareCheck(req.GoPath, req.Manifest, req.SkipNew, symbols)
}

// sanitizeIssueDetail limits the plugin-controlled text that goes into a public GitHub issue.
func sanitizeIssueDetail(detail string) string {
	detail = strings.ToValidUTF8(detail, "?")
	// The issue shows the detail in a code block: the plugin must not close it.
	detail = strings.ReplaceAll(detail, "`", "'")

	if len(detail) <= maxIssueDetailLength {
		return detail
	}

	cut := maxIssueDetailLength
	for cut > 0 && !utf8.RuneStart(detail[cut]) {
		cut--
	}

	return detail[:cut] + issueDetailTruncation
}

// limitedBuffer keeps the first max bytes written to it, and discards the other bytes.
// It never returns an error: an error would block the writes of the child process.
type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if remaining := b.max - b.buf.Len(); remaining > 0 {
		b.buf.Write(p[:min(len(p), remaining)])
	}

	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}
