package herdr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeHerdr answers herdr socket requests from canned results and records
// every request it gets.
type fakeHerdr struct {
	mu       sync.Mutex
	results  map[string]any
	requests []map[string]any
}

func (f *fakeHerdr) serve(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "herdr.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				sc := bufio.NewScanner(conn)
				for sc.Scan() {
					var req map[string]any
					_ = json.Unmarshal(sc.Bytes(), &req)
					f.mu.Lock()
					f.requests = append(f.requests, req)
					res, ok := f.results[req["method"].(string)]
					f.mu.Unlock()
					// An event first: the client must skip what is not its answer.
					_, _ = conn.Write([]byte(`{"event":"pane.updated"}` + "\n"))
					var msg map[string]any
					if ok {
						msg = map[string]any{"id": req["id"], "result": res}
					} else {
						msg = map[string]any{"id": req["id"], "result": map[string]any{"type": "ok"}}
					}
					b, _ := json.Marshal(msg)
					_, _ = conn.Write(append(b, '\n'))
					return // herdr answers one request per connection
				}
			}()
		}
	}()
	return path
}

func (f *fakeHerdr) calls(method string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, r := range f.requests {
		if r["method"] == method {
			out = append(out, r["params"].(map[string]any))
		}
	}
	return out
}

// fakeMultiversectl is a script that answers `universe which` with places
// for any path under dir and none for the rest, as multiversectl does.
func fakeMultiversectl(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "multiversectl")
	script := `#!/bin/sh
shift 5 # universe which -o json --
printf '['; sep=''; found=1
for p in "$@"; do
  case "$p" in
    ` + dir + `|` + dir + `/*) printf '%s{"path":"%s","multiverse":"acme","universe":"fix/login","state":"running","up":"3/3","dir":"` + dir + `"}' "$sep" "$p"; found=0;;
    *) printf '%s{"path":"%s"}' "$sep" "$p";;
  esac
  sep=','
done
printf ']\n'; exit $found
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestSyncLabelsPanesInAUniverseAndClearsTheRest(t *testing.T) {
	uni := "/home/u/multiverse/acme/universes/fix-login"
	f := &fakeHerdr{results: map[string]any{
		"pane.list": map[string]any{"panes": []any{
			map[string]any{"pane_id": "w1:p1", "foreground_cwd": uni + "/api", "cwd": "/elsewhere"},
			map[string]any{"pane_id": "w1:p2", "cwd": "/tmp"},
		}},
		"workspace.list": []any{
			map[string]any{"workspace_id": "w1", "worktree": map[string]any{"checkout_path": uni}},
			map[string]any{"workspace_id": "w2", "worktree": nil}, // no checkout: takes its panes' universe
		},
	}}
	f.results["pane.list"].(map[string]any)["panes"] = append(f.results["pane.list"].(map[string]any)["panes"].([]any),
		map[string]any{"pane_id": "w2:p1", "workspace_id": "w2", "cwd": uni + "/ui"},
		map[string]any{"pane_id": "w2:p2", "workspace_id": "w2", "cwd": "/tmp"},
	)
	var out bytes.Buffer
	p := Plugin{Multiversectl: fakeMultiversectl(t, uni), Socket: f.serve(t), Out: &out}
	if err := p.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	panes := f.calls("pane.report_metadata")
	if len(panes) != 4 {
		t.Fatalf("pane reports = %v", panes)
	}
	got := panes[0]["tokens"].(map[string]any)
	if panes[0]["pane_id"] != "w1:p1" || got["mv_universe"] != "fix/login" || got["mv_up"] != "3/3" || panes[0]["source"] != Source {
		t.Errorf("first pane, labelled from its foreground cwd: %v", panes[0])
	}
	if tok := panes[1]["tokens"].(map[string]any); tok["mv_universe"] != nil || len(tok) != len(Tokens) {
		t.Errorf("a pane in no multiverse clears every token: %v", tok)
	}
	ws := f.calls("workspace.report_metadata")
	if len(ws) != 2 || ws[0]["tokens"].(map[string]any)["mv_multiverse"] != "acme" {
		t.Fatalf("workspace reports = %v", ws)
	}
	if ws[1]["workspace_id"] != "w2" || ws[1]["tokens"].(map[string]any)["mv_universe"] != "fix/login" {
		t.Errorf("a workspace with no checkout takes its panes' universe: %v", ws[1])
	}
	if !strings.Contains(out.String(), "2 of 4 pane(s)") {
		t.Errorf("out = %q", out.String())
	}
}

func TestTokensForTheMultiverseRootSayRoot(t *testing.T) {
	tok := TokensFor(Place{Multiverse: "acme"})
	if tok["mv_universe"] != "root" || tok["mv_multiverse"] != "acme" || tok["mv_state"] != nil {
		t.Errorf("tokens = %v", tok)
	}
	if long := TokensFor(Place{Multiverse: "a", Universe: strings.Repeat("x", 100)}); len([]rune(long["mv_universe"].(string))) != 80 {
		t.Errorf("token not clipped to 80: %v", long["mv_universe"])
	}
}

func TestCallReturnsHerdrsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			line, _ := bufio.NewReader(conn).ReadBytes('\n')
			var req map[string]any
			_ = json.Unmarshal(line, &req)
			b, _ := json.Marshal(map[string]any{"id": req["id"], "error": map[string]any{"code": "not_found", "message": "no pane w9:p9"}})
			_, _ = conn.Write(append(b, '\n'))
			conn.Close()
		}
	}()
	c, err := Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = c.Call("pane.get", map[string]any{"pane_id": "w9:p9"})
	var he *Error
	if err == nil || !strings.Contains(err.Error(), "not_found") || !asError(err, &he) {
		t.Errorf("err = %v", err)
	}
}

func asError(err error, target **Error) bool {
	e, ok := err.(*Error)
	*target = e
	return ok
}

func TestMostOfPrefersAUniverseOnATie(t *testing.T) {
	root := Place{Multiverse: "acme"}
	uni := Place{Multiverse: "acme", Universe: "fix/login"}
	for i := 0; i < 20; i++ { // map order is random
		if got := mostOf(map[Place]int{root: 1, uni: 1}); got != uni {
			t.Fatalf("tie went to %+v", got)
		}
	}
	if got := mostOf(map[Place]int{root: 2, uni: 1}); got != root {
		t.Errorf("more votes win: %+v", got)
	}
}

func TestFocusedCwdReadsHerdrsContext(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", `{"focused_pane_id":"w1:p1","focused_pane_cwd":"/a/b","workspace_cwd":"/a"}`)
	if got := (Plugin{Socket: "/nonexistent"}).focusedCwd(); got != "/a/b" {
		t.Errorf("focusedCwd = %q", got)
	}
}

func TestOpenMakesAWorkspaceWithATabPerRepository(t *testing.T) {
	dir := "/m/acme/universes/fix-login"
	u := Universe{Multiverse: "acme", Name: "fix/login", State: "running", Up: "3/3", Dir: dir,
		Checkout: map[string]string{"web": dir + "/web", "api": dir + "/api"}}
	f := &fakeHerdr{results: map[string]any{
		"workspace.list":   map[string]any{"workspaces": []any{}},
		"workspace.create": map[string]any{"workspace": map[string]any{"workspace_id": "w5"}, "tab": map[string]any{"tab_id": "w5:t1"}},
	}}
	var out bytes.Buffer
	p := Plugin{Socket: f.serve(t), Out: &out}
	if err := p.Open(context.Background(), u, true); err != nil {
		t.Fatal(err)
	}
	ws := f.calls("workspace.create")
	if len(ws) != 1 || ws[0]["label"] != "fix/login" || ws[0]["cwd"] != dir+"/api" {
		t.Fatalf("workspace.create = %v", ws)
	}
	if r := f.calls("tab.rename"); len(r) != 1 || r[0]["tab_id"] != "w5:t1" || r[0]["label"] != "api" {
		t.Errorf("the first tab is the first repository: %v", r)
	}
	if tabs := f.calls("tab.create"); len(tabs) != 1 || tabs[0]["label"] != "web" || tabs[0]["cwd"] != dir+"/web" || tabs[0]["workspace_id"] != "w5" {
		t.Errorf("tab.create = %v", tabs)
	}
	if m := f.calls("workspace.report_metadata"); len(m) != 1 || m[0]["tokens"].(map[string]any)["mv_universe"] != "fix/login" {
		t.Errorf("the workspace is labelled with its universe: %v", m)
	}

	// Open again: the labelled workspace is focused, nothing is made.
	f.results["workspace.list"] = map[string]any{"workspaces": []any{map[string]any{
		"workspace_id": "w5", "label": "fix/login", "tokens": map[string]any{"mv_universe": "fix/login", "mv_multiverse": "acme"}}}}
	if err := p.Open(context.Background(), u, true); err != nil {
		t.Fatal(err)
	}
	if len(f.calls("workspace.create")) != 1 {
		t.Error("a second open made another workspace")
	}
	if fo := f.calls("workspace.focus"); len(fo) != 1 || fo[0]["workspace_id"] != "w5" {
		t.Errorf("workspace.focus = %v", fo)
	}
}

func TestTokensNameThePanesRepository(t *testing.T) {
	pl := Place{Path: "/m/acme/universes/fix-login/api/src", Multiverse: "acme", Universe: "fix/login", Dir: "/m/acme/universes/fix-login"}
	if got := TokensFor(pl)["mv_repo"]; got != "api" {
		t.Errorf("mv_repo = %v", got)
	}
	pl.Path = pl.Dir
	if got := TokensFor(pl)["mv_repo"]; got != nil {
		t.Errorf("the universe's own directory is in no repository: %v", got)
	}
}

func TestFindSaysWhichMultiverseWhenANameIsInTwo(t *testing.T) {
	all := []Universe{{Multiverse: "acme", Name: "main"}, {Multiverse: "globex", Name: "main"}}
	if _, err := find(all, "", "main"); err == nil || !strings.Contains(err.Error(), "-m") {
		t.Errorf("err = %v", err)
	}
	if u, err := find(all, "globex", "main"); err != nil || u.Multiverse != "globex" {
		t.Errorf("find -m globex = %+v, %v", u, err)
	}
}
