package commands

import (
	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/picodata/benchmark/stroppy/cmd/stroppy/commands/funcs"
	config2 "gitlab.com/picodata/benchmark/stroppy/pkg/database/config"
)

func newPayCommand(settings *config2.DatabaseSettings) *cobra.Command {
	payCmd := &cobra.Command{
		Use:     "pay",
		Aliases: []string{"transfer"},
		Short:   "Run the payments workload",
		Run: func(cmd *cobra.Command, args []string) {
			sum, err := funcs.Check(settings, nil)
			if err != nil {
				llog.Fatalf("%v", err)
			}

			llog.Infof("Initial balance: %v", sum)

			if err := funcs.Pay(settings); err != nil {
				llog.Fatalf("%v", err)
			}
			if settings.Check {
				balance, err := funcs.Check(settings, sum)
				if err != nil {
					llog.Fatalf("%v", err)
				}
				llog.Infof("Final balance: %v", balance)
			}
		},
	}

	payCmd.PersistentFlags().IntVarP(&settings.Count,
		"count", "n", settings.Count,
		"Number of transfers to make")
	payCmd.PersistentFlags().BoolVarP(&settings.ZIPFian,
		"zipfian", "z", settings.ZIPFian,
		"Use zipfian distribution for payments")
	payCmd.PersistentFlags().BoolVarP(&settings.Oracle,
		"oracle", "o", settings.Oracle,
		"Check all payments against the built-in oracle.")
	payCmd.PersistentFlags().BoolVarP(&settings.Check,
		"check", "", settings.Check,
		"Check the final balance to match the original one (set to false if benchmarking).")
	payCmd.PersistentFlags().BoolVarP(&settings.UseCustomTx,
		"tx", "t", settings.UseCustomTx,
		"Use custom implementation of atomic transactions (workaround for dbs without built-in ACID transactions).")

	return payCmd
}
