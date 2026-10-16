//go:build !linux

package core

import (
	"os"
	"syscall"
)

func evaluatorSysProcAttr() *syscall.SysProcAttr {
	return nil
}

// killEvaluator kills the evaluator process.
func killEvaluator(p *os.Process) error {
	if p == nil {
		return nil
	}

	return p.Kill()
}
