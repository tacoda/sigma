// Package exec runs shell commands behind the Executor port — the seam where
// sandboxing and worktree isolation plug in. Local is the default adapter and
// runs on the host with no isolation.
package exec

import (
	"context"
	osexec "os/exec"
	"time"
)

// Spec describes one command to run.
type Spec struct {
	Command string        // run via `bash -c`
	Timeout time.Duration // 0 means no timeout
}

// Executor runs a command and returns its combined stdout and stderr. A non-nil
// error means the command failed to start or exited non-zero; any output
// captured before the failure is still returned.
type Executor interface {
	Run(ctx context.Context, spec Spec) (output string, err error)
}

// Local runs commands on the host with no isolation.
type Local struct{}

func (Local) Run(ctx context.Context, spec Spec) (string, error) {
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	out, err := osexec.CommandContext(ctx, "bash", "-c", spec.Command).CombinedOutput()
	return string(out), err
}
