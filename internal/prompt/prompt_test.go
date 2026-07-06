package prompt

import (
	"errors"
	"testing"
)

type src string

func (s src) Contribute() (string, error) { return string(s), nil }

type errSrc struct{}

func (errSrc) Contribute() (string, error) { return "", errors.New("boom") }

func TestAssembleSkipsEmpty(t *testing.T) {
	got, err := Assemble(src("a"), src("  "), src("b"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "a\n\nb" {
		t.Errorf("got %q, want %q", got, "a\n\nb")
	}
}

func TestAssemblePropagatesError(t *testing.T) {
	if _, err := Assemble(src("a"), errSrc{}); err == nil {
		t.Error("want error, got nil")
	}
}
