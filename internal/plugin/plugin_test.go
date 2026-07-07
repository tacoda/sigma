package plugin

import (
	"testing"

	"github.com/tacoda/sigma/internal/hooks"
)

type fakePlugin struct{ name string }

func (f fakePlugin) Name() string { return f.name }
func (fakePlugin) Register(h *Host, _ Config) error {
	h.AddHook(hooks.Nop{})
	return nil
}

func TestMountKnownAndUnknown(t *testing.T) {
	Register(fakePlugin{"fake"})

	h, err := Mount([]string{"fake"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Hooks) != 1 {
		t.Errorf("mounted host has %d hooks, want 1", len(h.Hooks))
	}

	if _, err := Mount([]string{"ghost"}, nil); err == nil {
		t.Error("unknown plugin should error")
	}
}

func TestAvailableIncludesRegistered(t *testing.T) {
	Register(fakePlugin{"zeta"})
	found := false
	for _, n := range Available() {
		if n == "zeta" {
			found = true
		}
	}
	if !found {
		t.Errorf("Available() missing registered plugin: %v", Available())
	}
}
