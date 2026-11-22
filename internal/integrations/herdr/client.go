// Package herdr is multiversectl's integration with herdr (https://herdr.dev),
// the terminal agent multiplexer: what the multiversectl-herdr binary, run by
// the herdr plugin, does.
//
// It talks to herdr over its socket API (newline-delimited JSON requests
// {"id","method","params"} answered by {"id","result"} or {"id","error"}) and
// to multiversectl only through its command line and JSON output, never its
// packages, so it can be built here or move to a repository of its own.
package herdr

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// SocketPath is the socket of the herdr this process runs under: herdr sets
// HERDR_SOCKET_PATH for plugin commands and inside its panes; otherwise the
// default session's socket.
func SocketPath() string {
	if p := os.Getenv("HERDR_SOCKET_PATH"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "herdr", "herdr.sock")
}

// Client talks to herdr's socket. herdr answers one request per connection
// and then closes it, so each call dials afresh.
type Client struct {
	path string
	n    int
}

// Dial checks that herdr answers at path.
func Dial(path string) (*Client, error) {
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("herdr is not reachable at %s: %w", path, err)
	}
	conn.Close()
	return &Client{path: path}, nil
}

// Close is there for symmetry; a client holds no connection between calls.
func (c *Client) Close() error { return nil }

// Error is an error herdr answered with.
type Error struct {
	Method  string
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("herdr %s: %s (%s)", e.Method, e.Message, e.Code) }

// Call sends one request and returns its result.
func (c *Client) Call(method string, params any) (json.RawMessage, error) {
	c.n++
	id := fmt.Sprintf("mvctl_%d", c.n)
	if params == nil {
		params = map[string]any{}
	}
	req, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout("unix", c.path, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("herdr %s: %w", method, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return nil, fmt.Errorf("herdr %s: %w", method, err)
	}
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("herdr %s: %w", method, err)
		}
		var msg struct {
			ID     string          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *Error          `json:"error"`
		}
		if json.Unmarshal(line, &msg) != nil || msg.ID != id {
			continue // an event or another request's answer
		}
		if msg.Error != nil {
			msg.Error.Method = method
			return nil, msg.Error
		}
		return msg.Result, nil
	}
}

// Records is every JSON object in raw, at any depth, that has the key idKey:
// the panes of a pane.list ("pane_id"), the workspaces of a workspace.list
// ("workspace_id"). Reading them this way leaves room for herdr to wrap or
// nest its listings without this breaking.
func Records(raw json.RawMessage, idKey string) []map[string]any {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	var out []map[string]any
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if _, ok := t[idKey]; ok {
				out = append(out, t)
			}
			for _, c := range t {
				walk(c)
			}
		case []any:
			for _, c := range t {
				walk(c)
			}
		}
	}
	walk(v)
	return out
}

// Cwd is the working directory a pane or workspace record reports: the
// foreground process's when herdr knows it, which follows a `cd` in the pane.
//
// A workspace has no cwd of its own; when it is a git checkout, herdr gives
// the checkout's path as worktree.checkout_path.
func Cwd(rec map[string]any) string {
	for _, k := range []string{"foreground_cwd", "cwd"} {
		if s, ok := rec[k].(string); ok && s != "" {
			return s
		}
	}
	if wt, ok := rec["worktree"].(map[string]any); ok {
		if s, ok := wt["checkout_path"].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// ID is the record's idKey as a string.
func ID(rec map[string]any, idKey string) string {
	s, _ := rec[idKey].(string)
	return s
}
