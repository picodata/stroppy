package commands

import (
	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/picodata/stroppy/internal/deployment"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gopkg.in/inf.v0"
)

func newPopCommand(settings *config.Settings) *cobra.Command {
	popCmd := &cobra.Command{
		Use:     "pop",
		Aliases: []string{"populate"},
		Short:   "Create and populate the accounts database",
		Example: "./stroppy pop -c 100000000",

		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return initLogLevel(settings)
		},

		Run: func(cmd *cobra.Command, args []string) {
			if settings.TestSettings.UseCloudStroppy && settings.TestSettings.RunAsPod {
				llog.Fatalf("--use-cloud-stroppy and --run-as-pod flags specified at the same time")
			}

			if settings.Local && settings.TestSettings.RunAsPod {
				llog.Fatalf("--local and --run-as-pod flags specified at the same time")
			}

			if settings.TestSettings.UseCloudStroppy {
				sh, err := deployment.LoadState(settings)
				if err != nil {
					llog.Fatalf("shell load state failed: %v", err)
				}
				if err = sh.RunRemotePopTest(); err != nil {
					llog.Fatalf("test failed with error %v", err)
				}
			} else {
				p := createPayload(settings)
				err := p.StartStatisticsCollect()
				if err != nil {
					llog.Fatalf("get stat err %v", err)
				}

				if err = p.Pop(""); err != nil {
					llog.Fatalf("%v", err)
				}

				var balance *inf.Dec
				if balance, err = p.Check(nil); err != nil {
					llog.Fatalf("%v", err)
				}
				llog.Infof("Total balance: %v", balance)
			}
		},
	}

	popCmd.PersistentFlags().IntVarP(&settings.DatabaseSettings.Count,
		"count", "n",
		settings.DatabaseSettings.Count,
		"Number of accounts to create")

	popCmd.PersistentFlags().BoolVarP(&settings.TestSettings.RunAsPod,
		"run-as-pod", "",
		false,
		"run stroppy as in pod statement")

	popCmd.PersistentFlags().StringVarP(&settings.TestSettings.KubernetesMasterAddress,
		"kube-master-addr", "k",
		settings.TestSettings.KubernetesMasterAddress,
		"kubernetes master address")

	return popCmd
}
