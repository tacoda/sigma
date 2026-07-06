package exec

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSeatbeltProfile(t *testing.T) {
	p := seatbelt(Policy{}, "/work")
	if !strings.Contains(p, `(allow file-write* (subpath "/work"))`) {
		t.Error("profile should allow writes under the working dir")
	}
	if !strings.Contains(p, "(deny network*)") {
		t.Error("default profile should deny network")
	}
	if strings.Contains(seatbelt(Policy{AllowNetwork: true}, "/work"), "deny network") {
		t.Error("AllowNetwork should not deny network")
	}
}

func TestBwrapArgs(t *testing.T) {
	a := strings.Join(bwrapArgs(Policy{}, "/work"), " ")
	if !strings.Contains(a, "--unshare-net") {
		t.Error("default should unshare network")
	}
	if !strings.Contains(a, "--bind /work /work") {
		t.Error("working dir should be writable")
	}
	if strings.Contains(strings.Join(bwrapArgs(Policy{AllowNetwork: true}, "/work"), " "), "--unshare-net") {
		t.Error("AllowNetwork should not unshare network")
	}
}

func TestExtraWritableIncluded(t *testing.T) {
	dirs := writableDirs(Policy{Writable: []string{"/data"}}, "/work")
	var joined = strings.Join(dirs, " ")
	if !strings.Contains(joined, "/work") || !strings.Contains(joined, "/data") {
		t.Errorf("writableDirs = %v, want work + extra", dirs)
	}
}

// Integration: on macOS with sandbox-exec, a sandboxed command runs, can write
// inside its dir, and is denied writes outside it.
func TestSandboxDarwinConfines(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only integration test")
	}
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skip("sandbox-exec not available")
	}
	dir := t.TempDir()
	s := Sandbox{Dir: dir}
	ctx := context.Background()

	out, err := s.Run(ctx, Spec{Command: "echo hi"})
	if err != nil || strings.TrimSpace(out) != "hi" {
		t.Fatalf("sandboxed echo = %q, %v", out, err)
	}

	// Write outside any writable root ($HOME) must be denied.
	target := filepath.Join(os.Getenv("HOME"), ".sigma_sandbox_probe")
	if _, err := s.Run(ctx, Spec{Command: "echo x > " + target}); err == nil {
		os.Remove(target)
		t.Error("write outside the sandbox should fail")
	}
}
