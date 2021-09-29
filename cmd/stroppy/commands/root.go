/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package commands

import (
	"github.com/spf13/cobra"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/statistics"
)

func Execute() {
	settings := config.DefaultSettings()

	rootCmd := &cobra.Command{
		Use:   "stroppy [pop|pay|deploy|shell]",
		Short: "stroppy - a sample LWT application implementing an account ledger",
		Long: `
This program models an automatic banking system.  It implements 3 model
workloads, for populating the database with accounts, making transfers, and
checking correctness. It collects client-side metrics for latency and
bandwidth along the way.`,
		Version: "0.9",
		PersistentPreRunE: func(_ *cobra.Command, _ []string) (err error) {
			if err = initLogFacility(settings); err != nil {
				return
			}

			dbSettings := settings.DatabaseSettings
			if dbSettings.Workers > dbSettings.Count && dbSettings.Count > 0 {
				dbSettings.Workers = dbSettings.Count
			}

			statistics.StatsSetTotal(dbSettings.Count)
			return
		},
	}

	rootCmd.PersistentFlags().BoolVar(&settings.DestroyOnExit, "auto-destroy",
		settings.DestroyOnExit,
		"specify this key if you want to destroy deployed cluster when stroppy complete its own work")

	rootCmd.PersistentFlags().BoolVar(&settings.UseChaos, "use-chaos",
		settings.UseChaos,
		"install and run chaos-mesh on target cluster")

	rootCmd.PersistentFlags().BoolVar(&settings.Local, "local",
		settings.Local,
		"operate with local cluster")

	rootCmd.PersistentFlags().StringVarP(&settings.ChaosParameter,
		"chaos-parameter", "c",
		settings.ChaosParameter, "specify chaos parameter of an free form")

	rootCmd.PersistentFlags().StringVarP(&settings.LogLevel,
		"log-level", "v",
		settings.LogLevel,
		"Log level (trace, debug, info, warn, error, fatal, panic")

	rootCmd.PersistentFlags().StringVarP(&settings.DatabaseSettings.User,
		"user", "u",
		settings.DatabaseSettings.User,
		"Database user")

	rootCmd.PersistentFlags().StringVarP(&settings.DatabaseSettings.Password,
		"password", "p",
		settings.DatabaseSettings.Password,
		"Database password")

	rootCmd.PersistentFlags().StringVar(&settings.DatabaseSettings.DBType,
		"dbtype",
		settings.DatabaseSettings.DBType,
		"database type for deploy/ use")

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

	rootCmd.PersistentFlags().BoolVarP(&settings.TestSettings.UseCloudStroppy,
		"use-cloud-pod", "",
		false,
		"specify to use cloud stroppy pod instead of local generator")

	rootCmd.PersistentFlags().DurationVarP(&settings.DatabaseSettings.StatInterval,
		"stat-interval", "s",
		settings.DatabaseSettings.StatInterval,
		"interval by seconds for gettings db stats. Only fdb yet.")

	rootCmd.AddCommand(newPopCommand(settings),
		newPayCommand(settings),
		newDeployCommand(settings),
		newShellCommand(settings),
		newVersionCommand())

	_ = rootCmd.Execute()
}
