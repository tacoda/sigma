package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runTool(t *testing.T, tool Tool, in string) (string, error) {
	t.Helper()
	return tool.Run(context.Background(), json.RawMessage(in))
}

func TestRootedConfines(t *testing.T) {
	if _, err := rooted("/base", "../etc/passwd"); err == nil {
		t.Error("../ should escape and error")
	}
	if _, err := rooted("/base", "/etc/passwd"); err != nil {
		t.Errorf("absolute path should be re-rooted, not error: %v", err)
	}
	got, _ := rooted("/base", "a/b.txt")
	if got != "/base/a/b.txt" {
		t.Errorf("rooted = %q", got)
	}
	if got, _ := rooted("", "a/b.txt"); got != "a/b.txt" {
		t.Errorf("empty root should pass through, got %q", got)
	}
}

func TestWriteReadUnderRoot(t *testing.T) {
	root := t.TempDir()
	if _, err := runTool(t, WriteFile{Root: root}, `{"path":"sub/f.txt","content":"hello"}`); err != nil {
		t.Fatal(err)
	}
	// File lands under root, not cwd.
	if data, err := os.ReadFile(filepath.Join(root, "sub", "f.txt")); err != nil || string(data) != "hello" {
		t.Fatalf("file not written under root: %v %q", err, data)
	}
	out, err := runTool(t, ReadFile{Root: root}, `{"path":"sub/f.txt"}`)
	if err != nil || out != "hello" {
		t.Fatalf("read under root = %q, %v", out, err)
	}
}

func TestRootedToolsRejectEscape(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name string
		tool Tool
		in   string
	}{
		{"read", ReadFile{Root: root}, `{"path":"../../etc/passwd"}`},
		{"write", WriteFile{Root: root}, `{"path":"../evil","content":"x"}`},
		{"edit", EditFile{Root: root}, `{"path":"../evil","old_string":"a","new_string":"b"}`},
	} {
		if _, err := runTool(t, tc.tool, tc.in); err == nil {
			t.Errorf("%s: escape should error", tc.name)
		}
	}
}

func TestGlobGrepReturnRelative(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package x\n// TODO fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runTool(t, Glob{Root: root}, `{"pattern":"*.go"}`)
	if err != nil || out != "a.go" {
		t.Errorf("glob = %q, %v (want relative 'a.go')", out, err)
	}

	out, err = runTool(t, Grep{Root: root}, `{"pattern":"TODO"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "a.go:") {
		t.Errorf("grep = %q (want path relative to root)", out)
	}
}
