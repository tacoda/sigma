package eval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tacoda/sigma/internal/message"
)

func TestLiveRunnerRunsRecordsAndTraces(t *testing.T) {
	charter := t.TempDir() // empty charter: default config
	recDir := t.TempDir()

	// A scripted client stands in for the live model.
	client := &replayLLM{results: []*message.Result{
		writeFileResult("out.txt", "OK"),
		endTurnResult("done"),
	}}

	r, err := LiveRunner{Client: client, RecordDir: recDir}.Run(
		context.Background(),
		Variant{Name: "v", Charter: charter},
		Case{Name: "c", Prompt: "create out.txt"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(r.Dir)

	if r.Output != "done" {
		t.Errorf("output = %q, want %q", r.Output, "done")
	}
	// The agent wrote into the scratch workspace.
	if data, err := os.ReadFile(filepath.Join(r.Dir, "out.txt")); err != nil || string(data) != "OK" {
		t.Errorf("scratch file = %q, %v", data, err)
	}
	// The transcript was recorded for replay.
	if _, err := os.Stat(filepath.Join(recDir, "transcripts", "v-c.json")); err != nil {
		t.Errorf("transcript not recorded: %v", err)
	}
	// The trace captured the tool call.
	var sawTool bool
	for _, ev := range r.Trace {
		if ev.Kind == "PreToolUse" && ev.Tool == "write_file" {
			sawTool = true
		}
	}
	if !sawTool {
		t.Error("trace missing write_file event")
	}
}
