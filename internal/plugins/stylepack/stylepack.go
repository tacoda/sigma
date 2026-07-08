// Package stylepack is a built-in plugin that ships ready-made output styles
// (concise, explanatory, caveman) so they can be selected via outputStyle
// without authoring .sigma/styles files. A file style of the same name still
// overrides the built-in.
package stylepack

import (
	"github.com/tacoda/sigma/internal/plugin"
	"github.com/tacoda/sigma/internal/styles"
)

func init() { plugin.Register(plug{}) }

type plug struct{}

func (plug) Name() string { return "styles" }

// Register contributes the built-in styles to the styles registry. It uses no
// Host contributions — styles are selected by name via outputStyle, not mounted
// as tools or sources.
func (plug) Register(_ *plugin.Host, _ plugin.Config) error {
	for _, s := range builtin {
		styles.Register(s)
	}
	return nil
}

var builtin = []styles.Style{
	{
		Name:        "concise",
		Description: "Terse answers, no preamble",
		Body:        "Answer in the fewest words that are correct. Skip preamble, restatement, and filler. Prefer a direct answer, a short list, or code over prose.",
	},
	{
		Name:        "explanatory",
		Description: "Teach while you work",
		Body:        "As you work, briefly explain the why behind non-obvious decisions and trade-offs, so the reader learns. Keep explanations tight — a sentence or two per decision, not essays.",
	},
	{
		Name:        "caveman",
		Description: "Ultra-terse, drop filler words",
		Body:        "Talk like a smart caveman: drop articles and filler, use fragments, keep every technical term exact. Code, commands, and error messages stay verbatim. Compression must never lose meaning.",
	},
}
