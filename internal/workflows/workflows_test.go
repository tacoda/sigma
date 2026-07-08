package workflows

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadParsesStepsAndParallel(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.Join(".sigma", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `name: review-fix
steps:
  - name: review
    type: reviewer
    prompt: "Review {input}"
  - parallel:
      - { type: researcher, prompt: "A of {input}" }
      - { type: researcher, prompt: "B of {input}" }
  - name: fix
    prompt: "Fix {review}"
`
	if err := os.WriteFile(filepath.Join(dir, "review-fix.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	wf, ok := Load()["review-fix"]
	if !ok {
		t.Fatal("workflow not loaded")
	}
	if len(wf.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(wf.Steps))
	}
	if wf.Steps[0].Type != "reviewer" || wf.Steps[0].Prompt != "Review {input}" {
		t.Errorf("step 0 = %+v", wf.Steps[0])
	}
	if len(wf.Steps[1].Parallel) != 2 {
		t.Errorf("step 1 should have 2 parallel substeps, got %d", len(wf.Steps[1].Parallel))
	}
}

func TestNameDefaultsToFilename(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.Join(".sigma", "workflows")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "audit.yaml"), []byte("steps:\n  - prompt: go\n"), 0o644)
	if _, ok := Load()["audit"]; !ok {
		t.Errorf("name should default to filename: %v", Load().Names())
	}
}
