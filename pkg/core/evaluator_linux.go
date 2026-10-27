package core

import (
	"os"
	"syscall"
)

func evaluatorSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// The evaluator and its children are in a new process group: Piceus can kill all of them.
		Setpgid: true,
		// The evaluator dies if Piceus dies.
		Pdeathsig: syscall.SIGKILL,
	}
}

// killEvaluator kills the process group of the evaluator.
func killEvaluator(p *os.Process) error {
	if p == nil {
		return nil
	}

	return syscall.Kill(-p.Pid, syscall.SIGKILL)
}
