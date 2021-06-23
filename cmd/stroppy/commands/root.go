package commands

import (
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/statistics"
)

func Execute() {
	settings := config.DefaultSettings()

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
			dbSettings := settings.DatabaseSettings
			if l, err := llog.ParseLevel(dbSettings.LogLevel); err != nil {
				return merry.Wrap(err)
			} else {
				llog.SetLevel(l)
			}
			if dbSettings.Workers > dbSettings.Count && dbSettings.Count > 0 {
				dbSettings.Workers = dbSettings.Count
			}
			statistics.StatsSetTotal(dbSettings.Count)
			return nil
		},
	}

	rootCmd.PersistentFlags().BoolVar(&settings.UseChaos, "use-chaos",
		settings.UseChaos,
		"install and run chaos-mesh on target cluster")

	rootCmd.PersistentFlags().BoolVar(&settings.Local, "local",
		settings.Local,
		"operate with local cluster")

	rootCmd.PersistentFlags().StringVarP(&settings.ChaosParameter,
		"chaos-parameter", "c",
		settings.ChaosParameter, "specify chaos parameter of in free form")

	rootCmd.PersistentFlags().StringVarP(&settings.DatabaseSettings.LogLevel,
		"log-level", "v",
		settings.DatabaseSettings.LogLevel,
		"Log level (trace, debug, info, warn, error, fatal, panic")

	rootCmd.PersistentFlags().StringVarP(&settings.DatabaseSettings.User,
		"user", "u",
		settings.DatabaseSettings.User,
		"Database user")

	rootCmd.PersistentFlags().StringVarP(&settings.DatabaseSettings.Password,
		"password", "p",
		settings.DatabaseSettings.Password,
		"Database password")

	rootCmd.PersistentFlags().StringVarP(&settings.DatabaseSettings.DBType,
		"database", "d",
		settings.DatabaseSettings.DBType,
		"Database type, postgreSQL if not set.")

	rootCmd.PersistentFlags().StringVar(&settings.DatabaseSettings.DBURL,
		"url",
		settings.DatabaseSettings.DBURL,
		"Connection string, required flag")

	rootCmd.PersistentFlags().StringVar(&settings.WorkingDirectory,
		"dir",
		settings.WorkingDirectory,
		"Working directory, if not specified used ./benchmark/deploy")

	rootCmd.PersistentFlags().Float64VarP(&settings.DatabaseSettings.BanRangeMultiplier,
		"banRangeMultiplier", "r",
		settings.DatabaseSettings.BanRangeMultiplier,
		`
ban range multiplier (next brm) is a number that defines
the ratio of BAN (Bank Identifier Number) per BIC (Bank Identifier Code). 
The number of generated BICs is approximately equal to the square root of 'count'. 
The count of BANs is defined by the following formula: Nban = (Nbic * brm)/square(count). 
If Nban * Nbic > count we generate more (BIC, BAN) combinations 
than we saved during DB population process (that is achieved if brm > 1).
The recommended range of brm is from 1.01 to 1.1. 
The default value of banRangeMultipluer is 1.1.`)

	rootCmd.PersistentFlags().IntVarP(&settings.DatabaseSettings.Workers,
		"workers", "w",
		settings.DatabaseSettings.Workers,
		"Number of workers, 4 * NumCPU if not set.")

	rootCmd.AddCommand(newPopCommand(settings),
		newPayCommand(settings),
		newDeployCommand(settings))

	_ = rootCmd.Execute()
}
