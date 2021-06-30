package commands

import (
	"math/rand"
	"time"

	"gitlab.com/picodata/stroppy/internal/deployment"

	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/picodata/stroppy/pkg/database/config"
)

func newDeployCommand(settings *config.Settings) *cobra.Command {
	rand.Seed(time.Now().UnixNano())

	deploySettings := settings.DeploySettings
	deployCmd := &cobra.Command{
		Use:                    "dep",
		Aliases:                []string{"deploy"},
		SuggestFor:             []string{},
		Short:                  "Deploy infrastructure for tests",
		Long:                   "",
		Example:                "",
		ValidArgs:              []string{},
		ValidArgsFunction:      nil,
		Args:                   nil,
		ArgAliases:             []string{},
		BashCompletionFunction: "",
		Deprecated:             "",
		Annotations:            map[string]string{},
		Version:                "",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
		},
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return initLogLevel(settings)
		},
		PreRun: func(cmd *cobra.Command, args []string) {
		},
		PreRunE: nil,
		Run: func(cmd *cobra.Command, args []string) {
			d := deployment.CreateDeployment(settings)
			if err := d.Deploy(); err != nil {
				llog.Fatalf("status of exit: %v", err)
			}
			llog.Infoln("status of exit: success")
		},
		RunE:               nil,
		PostRun:            nil,
		PostRunE:           nil,
		PersistentPostRun:  nil,
		PersistentPostRunE: nil,
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: false,
		},
		TraverseChildren:           false,
		Hidden:                     false,
		SilenceErrors:              false,
		SilenceUsage:               false,
		DisableFlagParsing:         false,
		DisableAutoGenTag:          false,
		DisableFlagsInUseLine:      false,
		DisableSuggestions:         false,
		SuggestionsMinimumDistance: 0,
	}

	deployCmd.PersistentFlags().StringVar(&deploySettings.Provider,
		"cloud",
		deploySettings.Provider,
		"name of cloud provider")

	deployCmd.PersistentFlags().StringVar(&deploySettings.Flavor,
		"flavor",
		deploySettings.Flavor,
		"name of cluster configuration from templates.yml")

	deployCmd.PersistentFlags().IntVar(&deploySettings.Nodes,
		"nodes",
		deploySettings.Nodes,
		"count nodes of cluster")

	deployCmd.PersistentFlags().StringVar(&settings.DatabaseSettings.DBType,
		"dbtype",
		settings.DatabaseSettings.DBType,
		"database type for deploy")

	return deployCmd
}
