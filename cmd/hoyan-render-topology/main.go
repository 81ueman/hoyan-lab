package main

import (
	"os"

	"github.com/81ueman/hoyan-lab/internal/cli"
)

func main() {
	cmd := cli.NewRenderTopologyCommand()
	cmd.Use = "hoyan-render-topology"
	os.Exit(cli.Execute(cmd))
}
