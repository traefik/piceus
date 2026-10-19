package sandbox

import (
	"errors"
	"fmt"
	"os"

	seccomp "github.com/elastic/go-seccomp-bpf"
	"github.com/landlock-lsm/go-landlock/landlock"
	llsyscall "github.com/landlock-lsm/go-landlock/landlock/syscall"
)

// Available returns an error if the kernel cannot enforce the sandbox.
func Available() error {
	abi, err := llsyscall.LandlockGetABIVersion()
	if err != nil {
		return fmt.Errorf("getting landlock ABI version: %w", err)
	}

	if abi < 1 {
		return errors.New("landlock is not available")
	}

	// Landlock ABI v4 restricts TCP connections, and ABI v6 scopes signals.
	// Below v6, the plugin can signal any process of the same user, seccomp does not block kill.
	if abi < 6 {
		return fmt.Errorf("landlock ABI v6 or later is required, got v%d", abi)
	}

	if !seccomp.Supported() {
		return errors.New("seccomp is not available")
	}

	return nil
}

// Apply restricts all the threads of the current process, and the restrictions cannot be removed.
// After Apply, the process:
//   - can read and write only in workDir,
//   - can read only the system files that TLS, DNS and time zones need,
//   - cannot start a program, trace or read the memory of another process,
//   - can open TCP connections only to the ports 443 and 53,
//   - cannot send signals to processes outside of its Landlock domain.
func Apply(workDir string) error {
	if err := Available(); err != nil {
		return err
	}

	// System directories and files that TLS, DNS and time zones need.
	readOnlyDirs := []string{
		"/etc/ssl",
		"/etc/pki",
		"/etc/ca-certificates",
		"/usr/share/ca-certificates",
		"/usr/share/zoneinfo",
	}

	readOnlyFiles := []string{
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
		"/etc/services",
		"/etc/localtime",
	}

	// Available makes sure that Landlock ABI v6 or later is present: the file system, network and signal rules are enforced.
	// BestEffort only drops the rules of the ABI versions after v6.
	err := landlock.V10.BestEffort().Restrict(
		landlock.RWDirs(workDir),
		landlock.RODirs(readOnlyDirs...).IgnoreIfMissing(),
		landlock.ROFiles(readOnlyFiles...).IgnoreIfMissing(),
		landlock.RWFiles(os.DevNull),
		landlock.ConnectTCP(443),
		landlock.ConnectTCP(53),
		landlock.ConnectSendUDP(53),
		landlock.BindUDP(0),
	)
	if err != nil {
		return fmt.Errorf("applying landlock rules: %w", err)
	}

	// Stop the process from starting programs and from reading other processes.
	filter := seccomp.Filter{
		NoNewPrivs: true,
		Flag:       seccomp.FilterFlagTSync,
		Policy: seccomp.Policy{
			DefaultAction: seccomp.ActionAllow,
			Syscalls: []seccomp.SyscallGroup{
				{
					Action: seccomp.ActionErrno,
					Names:  []string{"execve", "execveat", "ptrace", "process_vm_readv", "process_vm_writev"},
				},
			},
		},
	}

	if err := seccomp.LoadFilter(filter); err != nil {
		return fmt.Errorf("applying seccomp filter: %w", err)
	}

	return nil
}
