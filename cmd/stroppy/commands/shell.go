package commands

import (
	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/picodata/stroppy/internal/deployment"
	"gitlab.com/picodata/stroppy/pkg/database/config"
)

func newShellCommand(settings *config.Settings) (shellCmd *cobra.Command) {
	shellCmd = &cobra.Command{
		Use:   "shell",
		Short: "Open shell to already deployed cluster",

		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return initLogFacility(settings)
		},

		Run: func(cmd *cobra.Command, args []string) {
			sh, err := deployment.LoadState(settings)
			if err != nil {
				llog.Fatalf("load shell error: %v", err)
			}
			if err = sh.ReadEvalPrintLoop(); err != nil {
				llog.Fatalf("repl return with error %v", err)
			}
		},
	}
	return
}
