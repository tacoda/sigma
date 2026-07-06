package session

import (
	"testing"

	"github.com/tacoda/sigma/internal/message"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	t.Chdir(t.TempDir())

	var s Store
	if _, ok, err := s.Load(); err != nil || ok {
		t.Fatalf("no session should exist yet (ok=%v err=%v)", ok, err)
	}
	want := []message.Message{
		message.UserText("hello"),
		{Role: "assistant", Content: []message.Block{{Type: "text", Text: "hi"}}},
	}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("session should exist after save")
	}
	if len(got) != 2 || got[0].Content[0].Text != "hello" || got[1].Content[0].Text != "hi" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
}
