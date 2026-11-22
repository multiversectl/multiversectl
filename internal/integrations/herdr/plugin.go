package herdr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Source is who the labels are from, in herdr's terms: a later report from
// the same source replaces an earlier one.
const Source = "plugin:multiversectl"

// Tokens are the sidebar tokens the labels set, used as $mv_universe and so
// on in herdr's ui.sidebar rows.
var Tokens = []string{"mv_universe", "mv_multiverse", "mv_state", "mv_up", "mv_repo"}

// Place is one entry of `multiversectl universe which -o json`: the contract
// between this integration and multiversectl.
type Place struct {
	Path       string `json:"path"`
	Multiverse string `json:"multiverse,omitempty"`
	Universe   string `json:"universe,omitempty"`
	State      string `json:"state,omitempty"`
	Up         string `json:"up,omitempty"`
	Dir        string `json:"dir,omitempty"`
}

// Plugin runs the plugin's commands.
type Plugin struct {
	// Multiversectl is the multiversectl binary to ask.
	Multiversectl string
	// Socket is herdr's socket.
	Socket string
	Out    io.Writer
	In     io.Reader
}

// Which asks multiversectl where each path is, in the order given. One call
// for all of them: each call reads the whole machine.
func (p Plugin) Which(ctx context.Context, paths []string) ([]Place, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, p.Multiversectl, append([]string{"universe", "which", "-o", "json", "--"}, paths...)...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) { // 1: none is in a multiverse
		return nil, fmt.Errorf("%s universe which: %w: %s", p.Multiversectl, err, strings.TrimSpace(errb.String()))
	}
	var places []Place
	if err := json.Unmarshal(out.Bytes(), &places); err != nil {
		return nil, fmt.Errorf("%s universe which: unexpected output: %w", p.Multiversectl, err)
	}
	if len(places) != len(paths) {
		return nil, fmt.Errorf("%s universe which: %d answers for %d paths", p.Multiversectl, len(places), len(paths))
	}
	return places, nil
}

// TokensFor are the sidebar tokens for a place; null clears a token an
// earlier sync set, so a pane that leaves a universe loses its label.
func TokensFor(pl Place) map[string]any {
	universe := pl.Universe
	if universe == "" && pl.Multiverse != "" {
		universe = "root"
	}
	vals := []string{universe, pl.Multiverse, pl.State, pl.Up, repoOf(pl)}
	out := make(map[string]any, len(Tokens))
	for i, k := range Tokens {
		if v := vals[i]; v != "" {
			out[k] = clip(v, 80) // herdr keeps 80 characters of a token
		} else {
			out[k] = nil
		}
	}
	return out
}

// repoOf is the repository a path in a universe is in: a universe keeps one
// checkout per repository, as <dir>/<repo>.
func repoOf(pl Place) string {
	if pl.Dir == "" || pl.Path == "" {
		return ""
	}
	rel, err := filepath.Rel(pl.Dir, pl.Path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	return strings.Split(rel, string(filepath.Separator))[0]
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

type target struct {
	method, idKey, id, cwd string
	workspace              string // a pane's workspace
}

// Sync labels every herdr pane and workspace with where its directory is. A
// workspace that is no checkout of its own takes the place most of its panes
// are in.
func (p Plugin) Sync(ctx context.Context) error {
	c, err := Dial(p.Socket)
	if err != nil {
		return err
	}
	defer c.Close()
	var targets []target
	panes, err := c.Call("pane.list", nil)
	if err != nil {
		return err
	}
	for _, rec := range Records(panes, "pane_id") {
		t := target{method: "pane.report_metadata", idKey: "pane_id", id: ID(rec, "pane_id"), cwd: Cwd(rec), workspace: ID(rec, "workspace_id")}
		if t.cwd == "" {
			t.cwd = paneCwd(c, t.id)
		}
		targets = append(targets, t)
	}
	spaces, err := c.Call("workspace.list", nil)
	if err != nil {
		return err
	}
	for _, rec := range Records(spaces, "workspace_id") {
		targets = append(targets, target{method: "workspace.report_metadata", idKey: "workspace_id", id: ID(rec, "workspace_id"), cwd: Cwd(rec)})
	}

	var paths []string
	for _, t := range targets {
		if t.cwd != "" {
			paths = append(paths, t.cwd)
		}
	}
	places, err := p.Which(ctx, paths)
	if err != nil {
		return err
	}
	at := map[string]Place{}
	for _, pl := range places {
		at[pl.Path] = pl
	}
	placeOf := func(cwd string) Place {
		if cwd == "" {
			return Place{}
		}
		abs, err := filepath.Abs(cwd)
		if err != nil {
			return Place{}
		}
		return at[abs]
	}

	// Where each workspace's panes are, for workspaces with no checkout.
	votes := map[string]map[Place]int{}
	for _, t := range targets {
		if t.idKey != "pane_id" {
			continue
		}
		if pl := placeOf(t.cwd); pl.Multiverse != "" {
			if votes[t.workspace] == nil {
				votes[t.workspace] = map[Place]int{}
			}
			pl.Path = ""
			votes[t.workspace][pl]++
		}
	}

	in, total := 0, 0
	for _, t := range targets {
		pl := placeOf(t.cwd)
		if t.idKey == "pane_id" {
			total++
			if pl.Universe != "" {
				in++
			}
		} else if pl.Multiverse == "" {
			pl = mostOf(votes[t.id])
		}
		params := map[string]any{t.idKey: t.id, "source": Source, "tokens": TokensFor(pl)}
		if _, err := c.Call(t.method, params); err != nil {
			return err
		}
	}
	fmt.Fprintf(p.Out, "%d of %d pane(s) in a universe\n", in, total)
	return nil
}

// mostOf is the place with the most votes; a tie goes to a universe over its
// multiverse's root, then by name, so the label does not flicker.
func mostOf(votes map[Place]int) Place {
	var best Place
	n := 0
	for pl, v := range votes {
		if v > n || v == n && better(pl, best) {
			best, n = pl, v
		}
	}
	return best
}

// better breaks a tie between two places.
func better(a, b Place) bool {
	if (a.Universe != "") != (b.Universe != "") {
		return a.Universe != ""
	}
	if a.Multiverse != b.Multiverse {
		return a.Multiverse < b.Multiverse
	}
	return a.Universe < b.Universe
}

func paneCwd(c *Client, id string) string {
	if id == "" {
		return ""
	}
	got, err := c.Call("pane.get", map[string]any{"pane_id": id})
	if err != nil {
		return ""
	}
	if recs := Records(got, "pane_id"); len(recs) > 0 {
		return Cwd(recs[0])
	}
	return ""
}

// focusedCwd is the directory of the pane a plugin command was started for.
// herdr's HERDR_PLUGIN_CONTEXT_JSON says it as focused_pane_cwd; the pane's id
// in HERDR_PANE_ID, which popups do not get, is the fallback.
func (p Plugin) focusedCwd() string {
	var ctx struct {
		FocusedPaneCwd string `json:"focused_pane_cwd"`
		WorkspaceCwd   string `json:"workspace_cwd"`
	}
	if raw := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"); raw != "" && json.Unmarshal([]byte(raw), &ctx) == nil {
		if ctx.FocusedPaneCwd != "" {
			return ctx.FocusedPaneCwd
		}
		if ctx.WorkspaceCwd != "" {
			return ctx.WorkspaceCwd
		}
	}
	c, err := Dial(p.Socket)
	if err != nil {
		return ""
	}
	return paneCwd(c, os.Getenv("HERDR_PANE_ID"))
}

// Popup is what the plugin's popup shows: the focused pane's universe, then
// `multiversectl status`, until enter closes it.
func (p Plugin) Popup(ctx context.Context, wait bool) error {
	if cwd := p.focusedCwd(); cwd != "" {
		if places, err := p.Which(ctx, []string{cwd}); err == nil && places[0].Universe != "" {
			pl := places[0]
			fmt.Fprintf(p.Out, "this pane: %s / %s  %s %s\n  %s\n\n", pl.Multiverse, pl.Universe, pl.State, pl.Up, pl.Dir)
		}
	}
	cmd := exec.CommandContext(ctx, p.Multiversectl, "status")
	cmd.Stdout, cmd.Stderr = p.Out, p.Out
	_ = cmd.Run()
	if wait {
		fmt.Fprint(p.Out, "\nenter to close ")
		_, _ = bufio.NewReader(p.In).ReadString('\n')
	}
	return nil
}

// Debug writes what herdr passes the plugin, and its pane and workspace
// lists, to debug.json in the plugin's state directory, and says where.
func (p Plugin) Debug() error {
	dump := map[string]any{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(k, "HERDR_") {
			continue
		}
		var j any
		if strings.HasSuffix(k, "_JSON") && json.Unmarshal([]byte(v), &j) == nil {
			dump[k] = j
		} else {
			dump[k] = v
		}
	}
	if c, err := Dial(p.Socket); err != nil {
		dump["_error"] = err.Error()
	} else {
		defer c.Close()
		for _, m := range []string{"pane.list", "workspace.list"} {
			if res, err := c.Call(m, nil); err != nil {
				dump["_"+m] = err.Error()
			} else {
				dump["_"+m] = res
			}
		}
	}
	dir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, "debug.json")
	b, _ := json.MarshalIndent(dump, "", "  ")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	fmt.Fprintln(p.Out, path)
	return nil
}
