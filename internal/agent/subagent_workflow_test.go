package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/tools"
	"github.com/tacoda/sigma/internal/workflows"
)

// echoClient replies with the last user message's text, so a step's output is
// its (substituted) prompt — letting the test verify placeholder chaining.
type echoClient struct{}

func (echoClient) Stream(_ context.Context, req message.Request, onText func(string)) (*message.Result, error) {
	text := ""
	if n := len(req.Messages); n > 0 {
		if c := req.Messages[n-1].Content; len(c) > 0 {
			text = c[0].Text
		}
	}
	if onText != nil {
		onText(text)
	}
	return &message.Result{Content: []message.Block{{Type: "text", Text: text}}, StopReason: "end_turn"}, nil
}

func workflowReg(wfs workflows.Set) *tools.Registry {
	return agent.WithSubagent(
		agent.Config{Client: echoClient{}},
		agent.SubagentOptions{Tools: func(string) []tools.Tool { return nil }, Workflows: wfs},
	)
}

func TestWorkflowChainsStepOutputs(t *testing.T) {
	reg := workflowReg(workflows.Set{"chain": {Name: "chain", Steps: []workflows.Step{
		{Name: "a", Prompt: "A:{input}"},
		{Name: "b", Prompt: "B:{a}"},
	}}})

	out, err := reg.Run(context.Background(), "workflow", json.RawMessage(`{"name":"chain","input":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "B:A:hi" {
		t.Errorf("workflow output = %q, want %q (chained substitution)", out, "B:A:hi")
	}
}

func TestWorkflowParallelStep(t *testing.T) {
	reg := workflowReg(workflows.Set{"par": {Name: "par", Steps: []workflows.Step{
		{Parallel: []workflows.Step{{Prompt: "P1:{input}"}, {Prompt: "P2:{input}"}}},
	}}})

	out, err := reg.Run(context.Background(), "workflow", json.RawMessage(`{"name":"par","input":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"P1:x", "P2:x", "## task 1", "## task 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("parallel output missing %q:\n%s", want, out)
		}
	}
}

func TestWorkflowUnknownErrors(t *testing.T) {
	reg := workflowReg(workflows.Set{"real": {Name: "real", Steps: []workflows.Step{{Prompt: "x"}}}})
	if _, err := reg.Run(context.Background(), "workflow", json.RawMessage(`{"name":"ghost"}`)); err == nil {
		t.Error("unknown workflow should error")
	}
}
