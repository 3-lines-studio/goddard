package main

import (
	"fmt"
	"os"

	"github.com/3-lines-studio/goddard/heimdall"
)

func main() {
	if err := heimdall.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "heimdall: %v\n", err)
		os.Exit(1)
	}
}
