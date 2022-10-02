/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package commands

import (
	"net/http"
	_ "net/http/pprof"
	"time"

	"gitlab.com/picodata/stroppy/internal/deployment"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/state"
	"gitlab.com/picodata/stroppy/pkg/statistics"

	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/inf.v0"
)

func newPayCommand(settings *config.Settings) *cobra.Command {
	payCmd := &cobra.Command{
		Use:     "pay",
		Aliases: []string{"transfer"},
		Short:   "Run the payments workload",

		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return initLogFacility(settings)
		},

		Run: func(cmd *cobra.Command, args []string) {
			statistics.StatsSetTotal(settings.DatabaseSettings.Count)

			if settings.EnableProfile {
				go func() {
					llog.Infoln(http.ListenAndServe("localhost:6060", nil))
				}()
			}
			if settings.TestSettings.UseCloudStroppy && settings.TestSettings.RunAsPod {
				llog.Fatalf("use-cloud-stroppy and run-as-pod flags specified at the same time")
			}

			if settings.Local && settings.TestSettings.RunAsPod {
				llog.Fatalf("--local and --run-as-pod flags specified at the same time")
			}

			if settings.TestSettings.UseCloudStroppy {
				sh, err := deployment.LoadState(settings)
				if err != nil {
					llog.Fatalf("deployment load state failed: %v", err)
				}
				if err = sh.RunRemotePayTest(); err != nil {
					llog.Fatalf("test failed with error %v", err)
				}
			} else {
				shellState := state.State{Settings: settings} //nolint
				dbPayload, err := createPayload(&shellState)
				if err != nil {
					llog.Fatalf("failed to create payload: %v", err)
				}

				if err = dbPayload.Connect(); err != nil {
					llog.Fatalf("failed to connect to cluster: %v", err)
				}

				if err = dbPayload.StartStatisticsCollect(
					settings.DatabaseSettings.StatInterval,
				); err != nil {
					llog.Fatalf("%v", err)
				}

				var sum *inf.Dec
				if sum, err = dbPayload.Check(nil); err != nil {
					llog.Fatalf("%v", err)
				}

				llog.Infof("Initial balance: %v", sum)

				beginTime := (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
				if err = dbPayload.Pay(&shellState); err != nil {
					llog.Fatalf("%v", err)
				}
				endTime := (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
				llog.Infof("pay test start time: '%d', end time: '%d'", beginTime, endTime)

				if settings.DatabaseSettings.Check {
					balance, err := dbPayload.Check(sum)
					if err != nil {
						llog.Fatalf("%v", err)
					}

					llog.Infof("Final balance: %v", balance)
				}
			}
		},
	}

	payCmd.PersistentFlags().IntVarP(&settings.DatabaseSettings.Count,
		"count", "n", settings.DatabaseSettings.Count,
		"Number of transfers to make")

	payCmd.PersistentFlags().BoolVarP(&settings.DatabaseSettings.Zipfian,
		"zipfian", "z", settings.DatabaseSettings.Zipfian,
		"Use zipfian distribution for payments")

	payCmd.PersistentFlags().BoolVarP(&settings.DatabaseSettings.Oracle,
		"oracle", "o", settings.DatabaseSettings.Oracle,
		"Check all payments against the built-in oracle.")

	payCmd.PersistentFlags().BoolVarP(&settings.DatabaseSettings.Check,
		"check", "", settings.DatabaseSettings.Check,
		"Check the final balance to match the original one (set to false if benchmarking).")

	payCmd.PersistentFlags().BoolVarP(&settings.DatabaseSettings.UseCustomTx,
		"tx", "t", settings.DatabaseSettings.UseCustomTx,
		"Use custom implementation of atomic transactions (workaround for dbs without built-in ACID transactions).")

	payCmd.PersistentFlags().StringVarP(&settings.TestSettings.KubernetesMasterAddress,
		"kube-master-addr", "k",
		settings.TestSettings.KubernetesMasterAddress,
		"kubernetes master address")

	payCmd.PersistentFlags().BoolVarP(&settings.TestSettings.RunAsPod,
		"run-as-pod", "",
		false,
		"run stroppy as in pod statement")

	return payCmd
}
