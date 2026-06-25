package session

import (
	"testing"

	"github.com/tacoda/sigma/internal/anthropic"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	t.Chdir(t.TempDir())

	if Exists() {
		t.Fatal("no session should exist yet")
	}
	want := []anthropic.Message{
		anthropic.UserText("hello"),
		{Role: "assistant", Content: []anthropic.Block{{Type: "text", Text: "hi"}}},
	}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	if !Exists() {
		t.Fatal("session should exist after save")
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Content[0].Text != "hello" || got[1].Content[0].Text != "hi" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
}
