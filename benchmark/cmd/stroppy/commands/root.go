package commands

import (
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	config2 "gitlab.com/picodata/benchmark/stroppy/pkg/database/config"
	"gitlab.com/picodata/benchmark/stroppy/pkg/statistics"
)

func Execute() {
	settings := config2.Defaults()

	rootCmd := &cobra.Command{
		Use:   "lightest [pop|pay|deploy]",
		Short: "lightest - a sample LWT application implementing an account ledger",
		Long: `
This program models an automatic banking system.  It implements 3 model
workloads, for populating the database with accounts, making transfers, and
checking correctness. It collects client-side metrics for latency and
bandwidth along the way.`,
		Version: "0.9",
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if l, err := llog.ParseLevel(settings.LogLevel); err != nil {
				return merry.Wrap(err)
			} else {
				llog.SetLevel(l)
			}
			if settings.Workers > settings.Count && settings.Count > 0 {
				settings.Workers = settings.Count
			}
			statistics.StatsSetTotal(settings.Count)
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVarP(&settings.LogLevel,
		"log-level", "v",
		settings.LogLevel,
		"Log level (trace, debug, info, warn, error, fatal, panic")

	rootCmd.PersistentFlags().StringVarP(&settings.User,
		"user", "u",
		settings.User,
		"Database user")

	rootCmd.PersistentFlags().StringVarP(&settings.Password,
		"password", "p",
		settings.Password,
		"Database password")

	rootCmd.PersistentFlags().StringVarP(&settings.DatabaseType,
		"database", "d",
		settings.DatabaseType,
		"Database type, postgreSQL if not set.")

	rootCmd.PersistentFlags().StringVar(&settings.DBURL,
		"url",
		settings.DBURL,
		"Connection string, required flag")

	rootCmd.PersistentFlags().Float64VarP(&settings.BanRangeMultiplier,
		"banRangeMultiplier", "r",
		settings.BanRangeMultiplier,
		`
ban range multiplier (next brm) is a number that defines
the ratio of BAN (Bank Identifier Number) per BIC (Bank Identifier Code). 
The number of generated BICs is approximately equal to the square root of 'count'. 
The count of BANs is defined by the following formula: Nban = (Nbic * brm)/square(count). 
If Nban * Nbic > count we generate more (BIC, BAN) combinations 
than we saved during DB population process (that is achieved if brm > 1).
The recommended range of brm is from 1.01 to 1.1. 
The default value of banRangeMultipluer is 1.1.`)

	rootCmd.PersistentFlags().IntVarP(&settings.Workers,
		"workers", "w",
		settings.Workers,
		"Number of workers, 4 * NumCPU if not set.")

	rootCmd.AddCommand(newPopulateCommand(settings),
		newPayCommand(settings),
		newDeployCommand(config2.DefaultsDeploy()))

	_ = rootCmd.Execute()
}
