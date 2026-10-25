package main

import (
	"os"

	"github.com/jayprio/beam-cli/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
