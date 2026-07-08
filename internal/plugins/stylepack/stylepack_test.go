package stylepack

import (
	"testing"

	"github.com/tacoda/sigma/internal/plugin"
	"github.com/tacoda/sigma/internal/styles"
)

func TestMountRegistersBuiltinStyles(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())

	if _, err := plugin.Mount([]string{"styles"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	set := styles.Load()
	for _, name := range []string{"concise", "explanatory", "caveman"} {
		if st, ok := set[name]; !ok || st.Body == "" {
			t.Errorf("built-in style %q not available after mount", name)
		}
	}
}
