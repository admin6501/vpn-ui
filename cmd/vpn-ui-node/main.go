// vpn-ui-node is the dedicated worker. No panel UI or public management listener.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/nodeagent"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--supports-uninstall" {
		fmt.Println("node-uninstall-v1")
		return
	}

	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version") {
		fmt.Println(config.GetVersion())
		return
	}
	if os.Geteuid() != 0 {
		log.Fatal("Run as root.")
	}
	command := "run"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	flags := flag.NewFlagSet(command, flag.ExitOnError)
	confirmed := flags.Bool("yes", false, "confirm node cleanup")
	path := flags.String("config", "/opt/vpn-ui-node/agent.json", "agent configuration")
	panel := flags.String("panel-url", "", "exact HTTPS panel URL, including its web path")
	pair := flags.String("pair-token", "", "single-use enrollment token")
	ca := flags.String("ca-file", "", "optional trusted CA certificate")
	if len(os.Args) > 1 {
		flags.Parse(os.Args[2:])
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	var err error
	switch command {
	case "enroll":
		err = nodeagent.Enroll(ctx, *path, *panel, *pair, *ca)
	case "uninstall":
		if !*confirmed {
			err = fmt.Errorf("use install-node.sh --uninstall to confirm removal")
		} else {
			err = nodeagent.Uninstall(*path)
		}
	case "run":
		var a *nodeagent.Agent
		a, err = nodeagent.New(*path)
		if err == nil {
			err = a.Run(ctx)
		}
	default:
		err = fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		log.Fatal(err)
	}
}
