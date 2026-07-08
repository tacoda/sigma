package eval

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads an experiment definition from a YAML file.
func Load(path string) (*Experiment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var e Experiment
	if err := yaml.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &e, nil
}
