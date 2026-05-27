package main

import (
	"os"

	"github.com/81ueman/hoyan-lab/internal/cli"
)

func main() {
	cmd := cli.NewLiveCheckCommand()
	cmd.Use = "hoyan-live-check"
	os.Exit(cli.Execute(cmd))
}
