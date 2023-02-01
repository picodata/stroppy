/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package commands

import (
	"net/http"
	_ "net/http/pprof"
	"time"

	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/picodata/stroppy/internal/deployment"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/state"
	"gitlab.com/picodata/stroppy/pkg/statistics"
	"gopkg.in/inf.v0"
)

func newPopCommand(settings *config.Settings) *cobra.Command {
	popCmd := &cobra.Command{
		Use:     "pop",
		Aliases: []string{"populate"},
		Short:   "Create and populate the accounts database",
		Example: "./stroppy pop -n 100000000",

		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return initLogFacility(settings)
		},

		Run: func(cmd *cobra.Command, args []string) {
			statistics.StatsSetTotal(int(settings.DatabaseSettings.Count))

			if settings.EnableProfile {
				go func() {
					llog.Infoln(http.ListenAndServe("localhost:6060", nil))
				}()
			}

			if settings.TestSettings.IsController() { //nolint
				sh, err := deployment.LoadState(settings)
				if err != nil {
					llog.Fatalf("deployment load state failed: %v", err)
				}
				if err = sh.RunRemotePopTest(); err != nil {
					llog.Fatalf("test failed with error %v", err)
				}
			} else {
				shellState := state.State{Settings: settings} //nolint
				dbPayload, err := createPayload(&shellState)
				if err != nil {
					llog.Fatalf("failed to create payload %v", err)
				}

				if err = dbPayload.Connect(); err != nil {
					llog.Fatalf("failed to connect to cluster: %v", err)
				}

				err = dbPayload.StartStatisticsCollect(settings.DatabaseSettings.StatInterval)
				if err != nil {
					llog.Fatalf("get stat err %v", err)
				}

				beginTime := (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
				if err = dbPayload.Pop(&shellState); err != nil {
					llog.Fatalf("%v", err)
				}
				endTime := (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
				llog.Infof("Pop test start time: '%d', end time: '%d'", beginTime, endTime)

				var balance *inf.Dec
				if balance, err = dbPayload.Check(nil); err != nil {
					llog.Fatalf("%v", err)
				}
				llog.Infof("Total balance: %v", balance)
			}
		},
	}

	popCmd.PersistentFlags().Uint64VarP(&settings.DatabaseSettings.Count,
		"count", "n",
		settings.DatabaseSettings.Count,
		"Number of accounts to create")

	popCmd.PersistentFlags().StringVarP(&settings.TestSettings.KubernetesMasterAddress,
		"kube-master-addr", "k",
		settings.TestSettings.KubernetesMasterAddress,
		"kubernetes master address")

	popCmd.PersistentFlags().BoolVarP(&settings.DatabaseSettings.Sharded,
		"sharded", "",
		false,
		"Use to populate accounts in sharded MongoDB cluster. Default false - populate accounts in MongoDB replicasets cluster")

	popCmd.PersistentFlags().StringVarP(
		&settings.TestSettings.RunType,
		"run-type", "",
		"controller",
		"set troppy run type [controller, client, or local]",
	)

	return popCmd
}
