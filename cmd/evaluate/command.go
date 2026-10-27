// Package evaluate implements the command that checks one plugin in a sandboxed process.
package evaluate

import (
	"fmt"
	"os"

	"github.com/traefik/piceus/pkg/core"
	"github.com/traefik/piceus/pkg/sandbox"
	"github.com/urfave/cli/v2"
)

// exitCodeFailure is the exit code when the evaluation cannot run.
const exitCodeFailure = 3

// Command creates the evaluate command.
// Piceus starts this command in a child process to run the code of one plugin.
// The command reads the request on stdin and writes the result on the file descriptor core.EvaluatorResultFD.
func Command() *cli.Command {
	return &cli.Command{
		Name:   "evaluate",
		Usage:  "Evaluate one plugin in a sandbox (internal use only)",
		Hidden: true,
		Action: func(cliCtx *cli.Context) error {
			result := os.NewFile(core.EvaluatorResultFD, "result")
			defer func() { _ = result.Close() }()

			err := core.RunEvaluation(cliCtx.Context, os.Stdin, result, sandbox.Apply)
			if err != nil {
				return cli.Exit(fmt.Sprintf("evaluating plugin: %v", err), exitCodeFailure)
			}

			return nil
		},
	}
}
