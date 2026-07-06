// Package prompt assembles the system prompt from independent context sources.
// Rules (CLAUDE.md) and the skill index are the sources today; memory and other
// contributors plug in the same way.
package prompt

import "strings"

// Source contributes one segment of the system prompt.
type Source interface {
	Contribute() (string, error)
}

// Assemble concatenates the non-empty contributions in order, separated by a
// blank line. The first source error is returned immediately.
func Assemble(sources ...Source) (string, error) {
	var parts []string
	for _, s := range sources {
		seg, err := s.Contribute()
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(seg) != "" {
			parts = append(parts, seg)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}
