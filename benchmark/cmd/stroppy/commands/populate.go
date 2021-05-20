package commands

import (
	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/picodata/stroppy/benchmark/cmd/stroppy/commands/funcs"
	"gitlab.com/picodata/stroppy/benchmark/pkg/database/config"
)

func newPopulateCommand(settings *config.DatabaseSettings) *cobra.Command {
	popCmd := &cobra.Command{
		Use:     "pop",
		Aliases: []string{"populate"},
		Short:   "Create and populate the accounts database",
		Example: "./lightest populate -n 100000000",

		Run: func(cmd *cobra.Command, args []string) {
			if err := funcs.Populate(settings); err != nil {
				llog.Fatalf("%v", err)
			}

			balance, err := funcs.Check(settings, nil)
			if err != nil {
				llog.Fatalf("%v", err)
			}
			llog.Infof("Total balance: %v", balance)
		},
	}

	popCmd.PersistentFlags().IntVarP(&settings.Count,
		"count", "n",
		settings.Count,
		"Number of accounts to create")
	// заполняем все поля, для неиспользуемых указвыаем nil, согласно требованиям линтера
	//nolint:gofumpt

	return popCmd
}
