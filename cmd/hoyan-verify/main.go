package main

import (
	"os"

	"github.com/81ueman/hoyan-lab/internal/cli"
)

func main() {
	cmd := cli.NewVerifyCommand()
	cmd.Use = "hoyan-verify"
	os.Exit(cli.Execute(cmd))
}
