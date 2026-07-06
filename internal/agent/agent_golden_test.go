package agent_test

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/tools"
)

var update = flag.Bool("update", false, "update golden files")

// TestRunGolden pins the agent loop's behavior: a two-turn conversation where
// the model requests a tool, the tool runs, and the model produces a final
// answer. The full message transcript is compared against a golden fixture so
// the Phase 1 ports refactor can prove it changed nothing.
func TestRunGolden(t *testing.T) {
	streamer := &fakeStreamer{script: []*message.Result{
		toolUseResult("I'll echo that.", "t1", "echo", `{"msg":"hello"}`),
		endTurnResult("Done: hello"),
	}}
	ui := &fakeUI{}
	approver := &allowApprover{}
	echo := &echoTool{}

	a := agent.New(agent.Config{
		Client:     streamer,
		Tools:      tools.NewRegistry(echo),
		Permission: approver,
		UI:         ui,
		Model:      "test-model",
		System:     "test-system",
	})

	if err := a.Run(context.Background(), "please echo hello"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Behavioral assertions independent of the golden file.
	if streamer.calls != 2 {
		t.Errorf("Stream calls = %d, want 2", streamer.calls)
	}
	if len(echo.inputs) != 1 || echo.inputs[0] != "hello" {
		t.Errorf("echo inputs = %v, want [hello]", echo.inputs)
	}
	if len(approver.asked) != 1 || approver.asked[0] != "echo" {
		t.Errorf("approver asked = %v, want [echo]", approver.asked)
	}
	if ui.text != "I'll echo that.Done: hello" {
		t.Errorf("ui text = %q", ui.text)
	}

	got, err := json.MarshalIndent(a.Snapshot(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	compareGolden(t, "transcript.golden.json", got)
}

func compareGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run `make golden` to create): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("transcript mismatch\n got: %s\nwant: %s", got, want)
	}
}
