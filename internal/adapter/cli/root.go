package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func Execute(cmd *cobra.Command) int {
	cmd.SetArgs(os.Args[1:])
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), err)
		var exitErr ExitError
		if asExitError(err, &exitErr) {
			return exitErr.Code
		}
		return 2
	}
	return 0
}

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "hoyan",
		Short:         "Hoyan WAN lab verification tools",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		NewCompareCommand(),
		NewCollectCommand(),
		NewTopologyCommand(),
		NewLabsCommand(),
		NewModelCommand(),
		NewIntentCommand(),
	)
	return cmd
}
