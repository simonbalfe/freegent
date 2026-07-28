package app

import (
	"github.com/simonbalfe/freegent/internal/api"
	"github.com/simonbalfe/freegent/internal/cli"
)

func Run(args []string) {
	if len(args) > 0 && (args[0] == "api" || args[0] == "serve") {
		api.Serve(args[1:])
		return
	}
	if len(args) > 0 && args[0] == "worker" {
		api.RunWorker(args[1:])
		return
	}
	cli.Run(args)
}
