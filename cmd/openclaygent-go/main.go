package main

import (
	"os"

	"github.com/simonbalfe/openclaygent-go/internal/claygent"
)

func main() {
	claygent.Run(os.Args[1:])
}
