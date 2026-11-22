// Command multiversectl-herdr is what the multiversectl plugin for herdr
// (https://herdr.dev) runs: it labels each herdr pane and workspace with the
// universe its directory is in, and shows universe status in a popup. It is a
// separate binary so multiversectl itself knows nothing of herdr; it asks
// multiversectl through `multiversectl universe which -o json`.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/multiversectl/multiversectl/internal/integrations/herdr"
)

var version = "dev"

const usage = `multiversectl-herdr labels herdr panes and workspaces with their multiversectl universe.

The multiversectl plugin for herdr runs it (herdr plugin install multiversectl/herdr-plugin);
you rarely need to.

usage:
  multiversectl-herdr sync      label every pane and workspace ($mv_universe, $mv_multiverse, $mv_state, $mv_up)
  multiversectl-herdr popup     the focused pane's universe, then multiversectl status, until enter
  multiversectl-herdr debug     write what herdr passes the plugin to debug.json in its state directory
  multiversectl-herdr version

environment:
  HERDR_SOCKET_PATH    herdr's socket (herdr sets it; default ~/.config/herdr/herdr.sock)
  MULTIVERSECTL_BIN    the multiversectl to ask (default: multiversectl on PATH)
`

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	noWait := flag.Bool("no-wait", false, "popup: exit without waiting for enter")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	bin := os.Getenv("MULTIVERSECTL_BIN")
	if bin == "" {
		bin = "multiversectl"
	}
	p := herdr.Plugin{Multiversectl: bin, Socket: herdr.SocketPath(), Out: os.Stdout, In: os.Stdin}

	var err error
	switch flag.Arg(0) {
	case "sync":
		err = p.Sync(ctx)
	case "popup":
		err = p.Popup(ctx, !*noWait)
	case "debug":
		err = p.Debug()
	case "version":
		fmt.Println(version)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "multiversectl-herdr:", err)
		os.Exit(1)
	}
}
