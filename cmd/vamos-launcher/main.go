package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	if err := maybeReexecManaged(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
