package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// placeOf is where a path is: in which multiverse, and in which of its
// universes, if any. A path in a multiverse but in none of its universes is
// in the multiverse's own checkouts.
type placeOf struct {
	Path       string `json:"path"`
	Multiverse string `json:"multiverse,omitempty"`
	Root       string `json:"root,omitempty"`
	Universe   string `json:"universe,omitempty"`
	State      string `json:"state,omitempty"`
	Up         string `json:"up,omitempty"`
	Branch     string `json:"branch,omitempty"`
	Dir        string `json:"dir,omitempty"`
}

// locate says where path is among the multiverses of st. The deepest match
// wins, so a universe beats the multiverse it is in.
func locate(st statusReport, path string) placeOf {
	p := placeOf{Path: path}
	if path == "" {
		return p
	}
	path = realPath(path)
	best := -1
	for _, mv := range st.Multiverses {
		if root := realPath(mv.Root); mv.Root != "" && within(path, root) && len(root) > best {
			best = len(root)
			p = placeOf{Path: p.Path, Multiverse: mv.Name, Root: mv.Root}
		}
		for _, u := range mv.Universes {
			if dir := realPath(u.Dir); u.Dir != "" && within(path, dir) && len(dir) > best {
				best = len(dir)
				p = placeOf{Path: p.Path, Multiverse: mv.Name, Root: mv.Root, Universe: u.Name, State: u.State, Up: u.Up, Branch: u.Branch, Dir: u.Dir}
			}
		}
	}
	return p
}

func within(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// realPath resolves links, so a path reached through a symlink still matches.
// A path that does not exist (yet, or any more) has its deepest existing
// parent resolved and the rest kept as written: on macOS /var is /private/var,
// and a folder not made yet must still compare equal to its multiverse.
func realPath(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	p = filepath.Clean(p)
	rest := ""
	for dir := p; ; dir = filepath.Dir(dir) {
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(r, rest)
		}
		if parent := filepath.Dir(dir); parent == dir {
			return p
		}
		rest = filepath.Join(filepath.Base(dir), rest)
	}
}

func (r runner) whichCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "which [path ...]",
		Short: "Which multiverse and universe a directory belongs to (the current one by default)",
		Long: "Says which multiverse each directory is in and, when it is in a universe's\n" +
			"checkouts, which universe, its state and how many of its services are up. It\n" +
			"exits 1 when none of them is in a multiverse. -o json is always an array, one\n" +
			"entry per path in the order given, for tools that label terminals or prompts.",
		Example: "  multiversectl universe which\n  multiversectl universe which ~/multiverse/acme/universes/fix-login/api /tmp -o json",
	}
	cmd.Flags().StringVarP(&format, "output", "o", "table", "output: table or json")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if format != "table" && format != "json" {
			return fmt.Errorf("--output must be table or json, not %q", format)
		}
		if len(args) == 0 {
			args = []string{"."}
		}
		st, err := r.collectStatus(cmd.Context(), false)
		if err != nil {
			return err
		}
		places := make([]placeOf, 0, len(args))
		found := false
		for _, a := range args {
			abs, err := filepath.Abs(a)
			if err != nil {
				return err
			}
			p := locate(st, abs)
			found = found || p.Multiverse != ""
			places = append(places, p)
		}
		if format == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(places); err != nil {
				return err
			}
		} else {
			for _, p := range places {
				prefix := ""
				if len(places) > 1 {
					prefix = p.Path + ": "
				}
				switch {
				case p.Universe != "":
					fmt.Printf("%s%s / %s  %s %s\n", prefix, p.Multiverse, r.out.Bold(p.Universe), p.State, p.Up)
				case p.Multiverse != "":
					fmt.Printf("%s%s  (its own checkouts, in no universe)\n", prefix, p.Multiverse)
				default:
					fmt.Printf("%s is in no multiverse\n", p.Path)
				}
			}
		}
		if !found {
			return errNotInMultiverse
		}
		return nil
	}
	return cmd
}
