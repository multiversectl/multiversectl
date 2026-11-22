package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocateFindsTheDeepestPlace(t *testing.T) {
	root := t.TempDir()
	uni := filepath.Join(root, "universes", "fix-login")
	if err := os.MkdirAll(filepath.Join(uni, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "here")
	if err := os.Symlink(filepath.Join(uni, "api"), link); err != nil {
		t.Fatal(err)
	}
	st := statusReport{Multiverses: []multiverseStatus{{
		Name: "acme", Root: root,
		Universes: []universeStatus{{Name: "fix/login", State: "running", Up: "3/3", Dir: uni}},
	}}}

	cases := []struct {
		path, multiverse, universe string
	}{
		{filepath.Join(uni, "api", "src"), "acme", "fix/login"}, // under a universe, even if it does not exist
		{uni, "acme", "fix/login"},                              // the universe directory itself
		{link, "acme", "fix/login"},                             // through a symlink
		{filepath.Join(root, "api"), "acme", ""},                // the multiverse's own checkout
		{uni + "-other", "acme", ""},                            // a sibling whose name starts the same
		{t.TempDir(), "", ""},                                   // nowhere
		{"", "", ""},
	}
	for _, c := range cases {
		got := locate(st, c.path)
		if got.Multiverse != c.multiverse || got.Universe != c.universe || got.Path != c.path {
			t.Errorf("locate(%q) = %+v, want multiverse %q universe %q", c.path, got, c.multiverse, c.universe)
		}
	}
	if got := locate(st, uni); got.State != "running" || got.Up != "3/3" || got.Dir != uni {
		t.Errorf("a universe's place carries its state: %+v", got)
	}
}
