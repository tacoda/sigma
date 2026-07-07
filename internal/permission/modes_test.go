package permission

import "testing"

// spyInner records delegated calls and returns a fixed answer.
type spyInner struct {
	called []string
	answer bool
}

func (s *spyInner) Allow(name, _ string) bool {
	s.called = append(s.called, name)
	return s.answer
}

func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{
		"acceptEdits": AcceptEdits,
		"plan":        Plan,
		"bypass":      Bypass,
		"":            Default,
		"garbage":     Default,
	} {
		if got := ParseMode(in); got != want {
			t.Errorf("ParseMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBypassAllowsAll(t *testing.T) {
	inner := &spyInner{answer: false}
	p := ForMode(Bypass, inner)
	if !p.Allow("bash", "rm -rf") {
		t.Error("bypass should allow")
	}
	if len(inner.called) != 0 {
		t.Error("bypass should not consult the inner gate")
	}
}

func TestPlanDeniesAll(t *testing.T) {
	inner := &spyInner{answer: true}
	p := ForMode(Plan, inner)
	if p.Allow("write_file", "x") {
		t.Error("plan should deny mutations")
	}
	if len(inner.called) != 0 {
		t.Error("plan should not consult the inner gate")
	}
}

func TestAcceptEditsApprovesEditsDelegatesRest(t *testing.T) {
	inner := &spyInner{answer: false}
	p := ForMode(AcceptEdits, inner)

	if !p.Allow("edit_file", "x") || !p.Allow("write_file", "x") {
		t.Error("acceptEdits should auto-approve edits")
	}
	if len(inner.called) != 0 {
		t.Error("edits should not hit the inner gate")
	}
	if p.Allow("bash", "x") {
		t.Error("bash should be delegated and denied by inner")
	}
	if len(inner.called) != 1 || inner.called[0] != "bash" {
		t.Errorf("expected bash delegated to inner, got %v", inner.called)
	}
}

func TestDefaultDelegates(t *testing.T) {
	inner := &spyInner{answer: true}
	if ForMode(Default, inner) != Allower(inner) {
		t.Error("default should be the inner gate itself")
	}
}
