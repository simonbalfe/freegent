package main

import (
	"fmt"
	"os"

	"github.com/simonbalfe/freegent/internal/api"
	"github.com/simonbalfe/freegent/internal/cli"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "api" {
		api.Serve(args[1:])
		return
	}
	if len(args) > 0 && args[0] == "worker" {
		api.RunWorker(args[1:])
		return
	}
	if err := cli.Run(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
