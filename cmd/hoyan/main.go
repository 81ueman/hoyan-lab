package main

import (
	"os"

	"github.com/81ueman/hoyan-lab/internal/adapter/cli"
)

func main() {
	os.Exit(cli.Execute(cli.NewRootCommand()))
}
