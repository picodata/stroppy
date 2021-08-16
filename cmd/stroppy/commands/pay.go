package commands

import (
	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/picodata/stroppy/internal/deployment"
	"gitlab.com/picodata/stroppy/pkg/database/config"
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
			if settings.TestSettings.UseCloudStroppy && settings.TestSettings.RunAsPod {
				llog.Fatalf("use-cloud-stroppy and run-as-pod flags specified at the same time")
			}

			if settings.Local && settings.TestSettings.RunAsPod {
				llog.Fatalf("--local and --run-as-pod flags specified at the same time")
			}

			if settings.TestSettings.UseCloudStroppy {
				sh, err := deployment.LoadState(settings)
				if err != nil {
					llog.Fatalf("shell load state failed: %v", err)
				}
				if err = sh.RunRemotePayTest(); err != nil {
					llog.Fatalf("test failed with error %v", err)
				}
			} else {
				p := createPayload(settings)
				err := p.Connect()
				if err != nil {
					llog.Fatalf("failed to connect to cluster: %v", err)
				}

				err = p.StartStatisticsCollect()
				if err != nil {
					llog.Fatalf("%v", err)
				}

				var sum *inf.Dec
				if sum, err = p.Check(nil); err != nil {
					llog.Fatalf("%v", err)
				}

				llog.Infof("Initial balance: %v", sum)

				if err = p.Pay(""); err != nil {
					llog.Fatalf("%v", err)
				}

				if settings.DatabaseSettings.Check {
					balance, err := p.Check(sum)
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
