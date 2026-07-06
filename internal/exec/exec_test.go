package exec

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLocalRun(t *testing.T) {
	out, err := Local{}.Run(context.Background(), Spec{Command: "echo hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(out) != "hi" {
		t.Errorf("output = %q, want %q", out, "hi")
	}
}

func TestLocalRunFailure(t *testing.T) {
	_, err := Local{}.Run(context.Background(), Spec{Command: "exit 3"})
	if err == nil {
		t.Error("want error for non-zero exit, got nil")
	}
}

func TestLocalRunTimeout(t *testing.T) {
	_, err := Local{}.Run(context.Background(), Spec{Command: "sleep 2", Timeout: 50 * time.Millisecond})
	if err == nil {
		t.Error("want timeout error, got nil")
	}
}
