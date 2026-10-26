package main

import (
	"os"

	"github.com/jayprio/beam-cli/cmd"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z"
// (see .github/workflows/release.yml); "dev" for a plain `go build`.
var version = "dev"

func main() {
	os.Exit(cmd.Execute(version))
}
