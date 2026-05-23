package main

import (
	"fmt"
	"os"

	"github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/rootcmd"
)

func main() {
	if err := rootcmd.NewCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
