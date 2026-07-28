package main

import (
	"os"

	"github.com/simonbalfe/freegent/internal/app"
)

func main() {
	app.Run(os.Args[1:])
}
