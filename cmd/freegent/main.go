package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/simonbalfe/freegent/internal/api"
	"github.com/simonbalfe/freegent/internal/cli"
	"github.com/simonbalfe/freegent/internal/codex"
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
	if len(args) > 0 && args[0] == "auth" {
		if err := authenticate(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := cli.Run(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func authenticate(args []string) error {
	defaultPath, err := codex.DefaultAuthPath()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("freegent auth", flag.ContinueOnError)
	filename := flags.String("file", defaultPath, "Codex credential file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: freegent auth [-file path]")
	}
	return codex.Authenticate(context.Background(), &http.Client{Timeout: 30 * time.Second}, *filename, os.Stdout)
}
