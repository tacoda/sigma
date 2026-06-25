package tui

import "testing"

// Pre-approved tools must short-circuit before any Program.Send, so Allow is
// safe to call with no running program.
func TestBridgePreApprove(t *testing.T) {
	b := &bridge{session: map[string]bool{}}
	b.preApprove([]string{"bash", "grep"})

	if !b.Allow("bash", "ls") {
		t.Error("pre-approved bash should allow without prompting")
	}
	if !b.Allow("grep", "x") {
		t.Error("pre-approved grep should allow without prompting")
	}
}
