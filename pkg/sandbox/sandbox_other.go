//go:build !linux

package sandbox

import "errors"

var errUnsupported = errors.New("the sandbox is only supported on Linux")

// Available returns an error if the kernel cannot enforce the sandbox.
func Available() error {
	return errUnsupported
}

// Apply restricts the current process. workDir is the only directory with write access.
func Apply(_ string) error {
	return errUnsupported
}
