package commands

import (
	"github.com/spf13/cobra"
	"gitlab.com/picodata/stroppy/pkg/database/config"
)

func newShellCommand(settings *config.Settings) (shellCmd *cobra.Command) {
	shellCmd = &cobra.Command{
		Use:   "shell",
		Short: "Open shell to already deployed cluster",

		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return initLogLevel(settings)
		},

		Run: func(cmd *cobra.Command, args []string) {
		},
	}
	return
}
