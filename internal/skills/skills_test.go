package skills

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	sk := parse("---\nname: greet\ndescription: say hi\n---\nThe body here.\nLine two.")
	if sk.Name != "greet" || sk.Description != "say hi" {
		t.Errorf("frontmatter = %+v", sk)
	}
	if sk.Body != "The body here.\nLine two." {
		t.Errorf("body = %q", sk.Body)
	}
}

func TestParseNoFrontmatter(t *testing.T) {
	sk := parse("just a body")
	if sk.Name != "" || sk.Body != "just a body" {
		t.Errorf("got %+v", sk)
	}
}

func TestParseBodyKeepsLeadingDash(t *testing.T) {
	sk := parse("---\nname: x\n---\n- item one\n- item two")
	if sk.Body != "- item one\n- item two" {
		t.Errorf("body = %q", sk.Body)
	}
}

func TestIndexEmpty(t *testing.T) {
	if (Set{}).Index() != "" {
		t.Error("empty set should produce empty index")
	}
}

func TestToolRun(t *testing.T) {
	set := Set{"greet": {Name: "greet", Description: "d", Body: "BODY"}}
	tool := NewTool(set)

	out, err := tool.Run(context.Background(), json.RawMessage(`{"name":"greet"}`))
	if err != nil || out != "BODY" {
		t.Errorf("out=%q err=%v", out, err)
	}

	_, err = tool.Run(context.Background(), json.RawMessage(`{"name":"nope"}`))
	if err == nil || !strings.Contains(err.Error(), "greet") {
		t.Errorf("expected unknown-skill error listing available, got %v", err)
	}
}
