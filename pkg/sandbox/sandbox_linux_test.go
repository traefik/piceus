package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	llsyscall "github.com/landlock-lsm/go-landlock/landlock/syscall"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const helperWorkDirEnv = "PICEUS_SANDBOX_TEST_WORKDIR"

// TestMain runs the sandbox helper when the test binary is started by TestApply.
func TestMain(m *testing.M) {
	if workDir := os.Getenv(helperWorkDirEnv); workDir != "" {
		os.Exit(runHelper(workDir))
	}

	os.Exit(m.Run())
}

func TestApply(t *testing.T) {
	if err := Available(); err != nil {
		t.Skipf("sandbox not available: %v", err)
	}

	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "file"), []byte("data"), 0o600))

	cmd := exec.Command(os.Args[0])
	cmd.Env = []string{helperWorkDirEnv + "=" + workDir}

	out, err := cmd.Output()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		t.Fatalf("helper failed: %v: %s", err, exitErr.Stderr)
	}
	require.NoError(t, err)

	var results map[string]string
	require.NoError(t, json.Unmarshal(out, &results))

	assert.Empty(t, results["read_workdir"], "read in the work directory")
	assert.Empty(t, results["write_workdir"], "write in the work directory")
	assert.Contains(t, results["read_passwd"], "permission denied")
	assert.Contains(t, results["read_parent"], "permission denied")
	assert.Contains(t, results["write_tmp"], "permission denied")
	assert.Contains(t, results["exec"], "operation not permitted")
	assert.Contains(t, results["start_process"], "operation not permitted")

	// The network rules need Landlock ABI v4 or later.
	if landlockNetSupported() {
		assert.Contains(t, results["connect_imds"], "permission denied")
		assert.Contains(t, results["connect_lo_80"], "permission denied")
		assert.NotContains(t, results["connect_lo_443"], "permission denied")
	}
}

// runHelper applies the sandbox, tries each operation and prints the result of each operation as JSON.
func runHelper(workDir string) int {
	if err := Apply(workDir); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		return 3
	}

	results := map[string]string{
		"read_workdir":   errString(readFile(filepath.Join(workDir, "file"))),
		"write_workdir":  errString(os.WriteFile(filepath.Join(workDir, "new"), []byte("x"), 0o600)),
		"read_passwd":    errString(readFile("/etc/passwd")),
		"read_parent":    errString(readFile("/proc/" + strconv.Itoa(os.Getppid()) + "/environ")),
		"write_tmp":      errString(os.WriteFile(filepath.Join(os.TempDir(), "piceus-sandbox-test"), []byte("x"), 0o600)),
		"exec":           errString(exec.Command("/bin/true").Run()),
		"start_process":  errString(startProcess()),
		"connect_imds":   errString(connect("169.254.169.254:80")),
		"connect_lo_80":  errString(connect("127.0.0.1:80")),
		"connect_lo_443": errString(connect("127.0.0.1:443")),
	}

	if err := json.NewEncoder(os.Stdout).Encode(results); err != nil {
		return 1
	}

	return 0
}

func readFile(path string) error {
	_, err := os.ReadFile(path)

	return err
}

func startProcess() error {
	p, err := os.StartProcess("/bin/sh", []string{"sh", "-c", "true"}, &os.ProcAttr{})
	if err != nil {
		return err
	}

	_, _ = p.Wait()

	return nil
}

func connect(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return err
	}

	return conn.Close()
}

func landlockNetSupported() bool {
	abi, err := llsyscall.LandlockGetABIVersion()

	return err == nil && abi >= 4
}

func errString(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}
