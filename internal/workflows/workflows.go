// Package workflows loads declarative multi-agent workflows: an ordered list of
// steps, each run by a sub-agent, with outputs chained into later steps.
//
// A workflow is a YAML file under .sigma/workflows/<name>.yaml:
//
//	name: review-fix
//	steps:
//	  - name: review
//	    type: reviewer                 # optional agent type
//	    prompt: "Review {input} for bugs."
//	  - parallel:                      # substeps run concurrently
//	      - { type: researcher, prompt: "explore A of {input}" }
//	      - { type: researcher, prompt: "explore B of {input}" }
//	  - name: fix
//	    prompt: "Fix the issues found:\n{review}"
//
// Prompts may reference {input} (the workflow argument), {prev} (the previous
// step's output), and {stepname} (a named step's output).
//
// Discovery (project overrides user on name conflict):
//
//	~/.sigma/workflows/<name>.yaml
//	./.sigma/workflows/<name>.yaml
package workflows

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Step is one workflow step: a single sub-agent, or a parallel group.
type Step struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Prompt   string `yaml:"prompt"`
	Parallel []Step `yaml:"parallel"`
}

// Workflow is a named sequence of steps.
type Workflow struct {
	Name  string `yaml:"name"`
	Steps []Step `yaml:"steps"`
}

// Set is the loaded workflows, keyed by name.
type Set map[string]Workflow

// Load reads workflows from the user then project directories.
func Load() Set {
	s := Set{}
	if home, err := os.UserHomeDir(); err == nil {
		s.loadDir(filepath.Join(home, ".sigma", "workflows"))
	}
	s.loadDir(filepath.Join(".sigma", "workflows"))
	return s
}

// Names returns the workflow names, sorted.
func (s Set) Names() []string {
	names := make([]string, 0, len(s))
	for n := range s {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (s Set) loadDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var wf Workflow
		if yaml.Unmarshal(data, &wf) != nil {
			continue
		}
		if wf.Name == "" {
			wf.Name = strings.TrimSuffix(e.Name(), ".yaml")
		}
		s[wf.Name] = wf
	}
}
