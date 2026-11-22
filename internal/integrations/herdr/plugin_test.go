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
		"workspace.list": []any{map[string]any{"workspace_id": "w1", "cwd": uni}},
	}}
	var out bytes.Buffer
	p := Plugin{Multiversectl: fakeMultiversectl(t, uni), Socket: f.serve(t), Out: &out}
	if err := p.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	panes := f.calls("pane.report_metadata")
	if len(panes) != 2 {
		t.Fatalf("pane reports = %v", panes)
	}
	got := panes[0]["tokens"].(map[string]any)
	if panes[0]["pane_id"] != "w1:p1" || got["mv_universe"] != "fix/login" || got["mv_up"] != "3/3" || panes[0]["source"] != Source {
		t.Errorf("first pane, labelled from its foreground cwd: %v", panes[0])
	}
	if tok := panes[1]["tokens"].(map[string]any); tok["mv_universe"] != nil || len(tok) != len(Tokens) {
		t.Errorf("a pane in no multiverse clears every token: %v", tok)
	}
	if ws := f.calls("workspace.report_metadata"); len(ws) != 1 || ws[0]["tokens"].(map[string]any)["mv_multiverse"] != "acme" {
		t.Errorf("workspace reports = %v", ws)
	}
	if !strings.Contains(out.String(), "1 of 2 pane(s)") {
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
		conn, _ := ln.Accept()
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadBytes('\n')
		var req map[string]any
		_ = json.Unmarshal(line, &req)
		b, _ := json.Marshal(map[string]any{"id": req["id"], "error": map[string]any{"code": "not_found", "message": "no pane w9:p9"}})
		_, _ = conn.Write(append(b, '\n'))
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
