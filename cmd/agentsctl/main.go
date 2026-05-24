package main

import (
	"fmt"
	"os"

	"github.com/CoreyCole/vamos/pkg/ctl/verifycmd"
	"github.com/CoreyCole/vamos/pkg/ctl/workspacecmd"
)

const usageExitCode = 2

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "verify" && os.Args[2] == "workspaces" {
		if err := verifycmd.Main(os.Args[3:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "workspace" {
		if err := workspacecmd.Main(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	fmt.Fprintln(
		os.Stderr,
		"usage: agentsctl verify workspaces [flags]\n       agentsctl workspace <create|status|logs|doctor|restart|register-current> [flags]\n\nPrefer: vamos ctl ...",
	)
	os.Exit(usageExitCode)
}
