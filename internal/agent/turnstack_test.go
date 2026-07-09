package agent

import (
	"strings"
	"testing"
)

func TestTurnStackNames(t *testing.T) {
	if got := strings.Join(TurnStack(), ","); got != "compaction,prompt-gate,loop" {
		t.Errorf("turn stack = %q, want compaction,prompt-gate,loop", got)
	}
}
