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
  multiversectl-herdr open [-m multiverse] <universe>   open a universe as a workspace, in its folder
  multiversectl-herdr open --all                        open every universe
  multiversectl-herdr pick      list the universes and open the one chosen (the plugin's popup)
  multiversectl-herdr sync      label every pane and workspace ($mv_universe, $mv_multiverse, $mv_state, $mv_up)
  multiversectl-herdr popup [--no-wait]   the focused pane's universe, then multiversectl status, until enter
  multiversectl-herdr debug     write what herdr passes the plugin to debug.json in its state directory
  multiversectl-herdr version

environment:
  HERDR_SOCKET_PATH    herdr's socket (herdr sets it; default ~/.config/herdr/herdr.sock)
  MULTIVERSECTL_BIN    the multiversectl to ask (default: multiversectl on PATH)
`

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	bin := os.Getenv("MULTIVERSECTL_BIN")
	if bin == "" {
		bin = "multiversectl"
	}
	p := herdr.Plugin{Multiversectl: bin, Socket: herdr.SocketPath(), Out: os.Stdout, In: os.Stdin}

	// Each command parses its own flags, after its name.
	cmd := flag.NewFlagSet(flag.Arg(0), flag.ExitOnError)
	cmd.Usage = flag.Usage
	noWait := cmd.Bool("no-wait", false, "popup: exit without waiting for enter")
	mv := cmd.String("m", "", "open: the multiverse the universe is in, when two have one of that name")
	all := cmd.Bool("all", false, "open: every universe")
	if flag.NArg() > 0 {
		_ = cmd.Parse(flag.Args()[1:])
	}

	var err error
	switch flag.Arg(0) {
	case "open":
		if *all == (cmd.NArg() == 1) || cmd.NArg() > 1 {
			fmt.Fprintln(os.Stderr, "multiversectl-herdr open: name one universe, or --all")
			os.Exit(2)
		}
		err = p.OpenNamed(ctx, *mv, cmd.Arg(0), *all)
	case "pick":
		err = p.Pick(ctx)
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
