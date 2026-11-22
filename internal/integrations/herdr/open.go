package herdr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Universe is a universe as `multiversectl universe ls -o json` gives it,
// with the multiverse it is in: what opening it as a workspace needs.
// Checkout is each repository's checkout, under Dir.
type Universe struct {
	Multiverse string            `json:"-"`
	Name       string            `json:"name"`
	State      string            `json:"state"`
	Up         string            `json:"up"`
	Dir        string            `json:"dir"`
	Checkout   map[string]string `json:"checkout"`
}

func (p Plugin) run(ctx context.Context, args ...string) ([]byte, error) {
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, p.Multiversectl, args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", p.Multiversectl, strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// Universes are every universe of every multiverse on this machine.
func (p Plugin) Universes(ctx context.Context) ([]Universe, error) {
	raw, err := p.run(ctx, "status", "-o", "json")
	if err != nil {
		return nil, err
	}
	var st struct {
		Multiverses []struct {
			Name  string `json:"name"`
			Error string `json:"error"`
		} `json:"multiverses"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("multiversectl status: unexpected output: %w", err)
	}
	var all []Universe
	for _, mv := range st.Multiverses {
		if mv.Error != "" {
			continue
		}
		raw, err := p.run(ctx, "universe", "ls", "-o", "json", "-m", mv.Name)
		if err != nil {
			return nil, err
		}
		var ls struct {
			Universes []Universe `json:"universes"`
		}
		if err := json.Unmarshal(raw, &ls); err != nil {
			return nil, fmt.Errorf("multiversectl universe ls -m %s: unexpected output: %w", mv.Name, err)
		}
		for _, u := range ls.Universes {
			u.Multiverse = mv.Name
			all = append(all, u)
		}
	}
	return all, nil
}

// find is the universe called name, in multiverse mv when that is given.
func find(all []Universe, mv, name string) (Universe, error) {
	var hits []Universe
	for _, u := range all {
		if u.Name == name && (mv == "" || u.Multiverse == mv) {
			hits = append(hits, u)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return Universe{}, fmt.Errorf("no universe %q; multiversectl status lists them", name)
	}
	var in []string
	for _, u := range hits {
		in = append(in, u.Multiverse)
	}
	return Universe{}, fmt.Errorf("universe %q is in %s; say which with -m", name, strings.Join(in, " and "))
}

// workspaceOf is the herdr workspace already open for u: the one whose labels
// say it is u.
func workspaceOf(c *Client, u Universe) (string, error) {
	res, err := c.Call("workspace.list", nil)
	if err != nil {
		return "", err
	}
	for _, w := range Records(res, "workspace_id") {
		tok, _ := w["tokens"].(map[string]any)
		if tok["mv_universe"] == clip(u.Name, 80) && tok["mv_multiverse"] == u.Multiverse && w["label"] == u.Name {
			return ID(w, "workspace_id"), nil
		}
	}
	return "", nil
}

// Open shows universe u as a herdr workspace named after it, with one tab,
// "universe", in the universe's folder: the parent of every repository's
// checkout. A workspace already open for it is focused instead.
func (p Plugin) Open(ctx context.Context, u Universe, focus bool) error {
	c, err := Dial(p.Socket)
	if err != nil {
		return err
	}
	if id, err := workspaceOf(c, u); err != nil {
		return err
	} else if id != "" {
		if focus {
			_, err = c.Call("workspace.focus", map[string]any{"workspace_id": id})
		}
		fmt.Fprintf(p.Out, "%s / %s is open\n", u.Multiverse, u.Name)
		return err
	}

	res, err := c.Call("workspace.create", map[string]any{"cwd": u.Dir, "label": u.Name, "focus": focus})
	if err != nil {
		return err
	}
	var created struct {
		Workspace struct {
			ID string `json:"workspace_id"`
		} `json:"workspace"`
		Tab struct {
			ID string `json:"tab_id"`
		} `json:"tab"`
	}
	if err := json.Unmarshal(res, &created); err != nil || created.Workspace.ID == "" {
		return fmt.Errorf("herdr workspace.create: unexpected answer %s", res)
	}
	if created.Tab.ID != "" {
		if _, err := c.Call("tab.rename", map[string]any{"tab_id": created.Tab.ID, "label": "universe"}); err != nil {
			return err
		}
	}
	tokens := TokensFor(Place{Multiverse: u.Multiverse, Universe: u.Name, State: u.State, Up: u.Up, Dir: u.Dir})
	if _, err := c.Call("workspace.report_metadata", map[string]any{"workspace_id": created.Workspace.ID, "source": Source, "tokens": tokens}); err != nil {
		return err
	}
	fmt.Fprintf(p.Out, "opened %s / %s in %s\n", u.Multiverse, u.Name, u.Dir)
	return nil
}

// OpenNamed opens the universe called name (in multiverse mv, if given), or
// every universe when all is set.
func (p Plugin) OpenNamed(ctx context.Context, mv, name string, all bool) error {
	us, err := p.Universes(ctx)
	if err != nil {
		return err
	}
	if all {
		for i, u := range us {
			if err := p.Open(ctx, u, i == 0); err != nil {
				return err
			}
		}
		if len(us) == 0 {
			fmt.Fprintln(p.Out, "no universes; multiversectl universe new makes one")
		}
		return nil
	}
	u, err := find(us, mv, name)
	if err != nil {
		return err
	}
	return p.Open(ctx, u, true)
}

// Pick lists every universe, numbered, and opens the one chosen: what the
// plugin's "open universe" popup runs.
func (p Plugin) Pick(ctx context.Context) error {
	us, err := p.Universes(ctx)
	if err != nil {
		return err
	}
	if len(us) == 0 {
		fmt.Fprint(p.Out, "no universes; multiversectl universe new makes one\n\nenter to close ")
		_, _ = bufio.NewReader(p.In).ReadString('\n')
		return nil
	}
	fmt.Fprintln(p.Out, "open a universe as a workspace, in its folder")
	fmt.Fprintln(p.Out)
	for i, u := range us {
		fmt.Fprintf(p.Out, "  %2d  %s %-36s %-8s %5s  %s\n", i+1, stateIcon(u.State), u.Name, u.State, u.Up, u.Multiverse)
	}
	fmt.Fprintf(p.Out, "\n   a  all of them\n\nnumber (enter to close): ")
	line, _ := bufio.NewReader(p.In).ReadString('\n')
	line = strings.TrimSpace(line)
	switch {
	case line == "":
		return nil
	case line == "a":
		for i, u := range us {
			if err := p.Open(ctx, u, i == 0); err != nil {
				return err
			}
		}
		return nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(us) {
		return fmt.Errorf("%q is not one of 1 to %d", line, len(us))
	}
	return p.Open(ctx, us[n-1], true)
}

func stateIcon(state string) string {
	switch state {
	case "running":
		return "●"
	case "partial":
		return "◐"
	case "stopped":
		return "○"
	}
	return "◌"
}
