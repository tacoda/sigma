package exec

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Policy describes what a sandboxed command may touch.
type Policy struct {
	AllowNetwork bool     // permit network access
	Writable     []string // extra writable directories beyond the working dir and temp
}

// Sandbox is an Executor that confines commands with the OS sandbox: seatbelt
// (sandbox-exec) on macOS, bubblewrap (bwrap) on Linux. Writes are limited to
// the working directory, temp, and Policy.Writable; network is denied unless
// Policy.AllowNetwork. It fails closed: if the backend is missing or the OS is
// unsupported, Run returns an error rather than running unconfined.
type Sandbox struct {
	Dir    string
	Policy Policy
}

func (s Sandbox) Run(ctx context.Context, spec Spec) (string, error) {
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	dir := absDir(firstNonEmpty(spec.Dir, s.Dir))
	name, args, err := s.wrap(dir, spec.Command)
	if err != nil {
		return "", err
	}
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// wrap builds the sandbox command line for the current OS.
func (s Sandbox) wrap(dir, command string) (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		if _, err := osexec.LookPath("sandbox-exec"); err != nil {
			return "", nil, fmt.Errorf("sandbox: sandbox-exec not found")
		}
		return "sandbox-exec", []string{"-p", seatbelt(s.Policy, dir), "bash", "-c", command}, nil
	case "linux":
		if _, err := osexec.LookPath("bwrap"); err != nil {
			return "", nil, fmt.Errorf("sandbox: bwrap (bubblewrap) not found; install it or disable sandbox")
		}
		return "bwrap", append(bwrapArgs(s.Policy, dir), "bash", "-c", command), nil
	default:
		return "", nil, fmt.Errorf("sandbox: unsupported on %s", runtime.GOOS)
	}
}

// writableDirs is the set a sandboxed command may write to.
func writableDirs(pol Policy, dir string) []string {
	dirs := []string{dir, "/tmp", "/private/tmp", "/var/folders", "/private/var/folders", "/dev"}
	for _, d := range pol.Writable {
		dirs = append(dirs, absDir(d))
	}
	return dirs
}

// seatbelt builds a macOS sandbox profile: read/execute anywhere, write only to
// the allowed dirs, network denied unless permitted.
func seatbelt(pol Policy, dir string) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n(deny file-write*)\n")
	for _, d := range writableDirs(pol, dir) {
		fmt.Fprintf(&b, "(allow file-write* (subpath %q))\n", d)
	}
	if !pol.AllowNetwork {
		b.WriteString("(deny network*)\n")
	}
	return b.String()
}

// bwrapArgs builds bubblewrap arguments: whole fs read-only, allowed dirs
// writable, network unshared unless permitted.
func bwrapArgs(pol Policy, dir string) []string {
	args := []string{"--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc"}
	for _, d := range writableDirs(pol, dir) {
		if d == "/dev" {
			continue // already provided by --dev
		}
		args = append(args, "--bind", d, d)
	}
	if !pol.AllowNetwork {
		args = append(args, "--unshare-net")
	}
	return append(args, "--chdir", dir)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func absDir(dir string) string {
	if dir == "" {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
		return "."
	}
	if a, err := filepath.Abs(dir); err == nil {
		return a
	}
	return dir
}
