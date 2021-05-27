package commands

import (
	"math/rand"
	"time"

	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.com/picodata/stroppy/cmd/stroppy/commands/funcs"
	"gitlab.com/picodata/stroppy/pkg/database/config"
)

func newDeployCommand(deploySettings *config.DeploySettings) *cobra.Command {
	rand.Seed(time.Now().UnixNano())

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
		PersistentPreRunE: nil,
		PreRun: func(cmd *cobra.Command, args []string) {
		},
		PreRunE: nil,
		Run: func(cmd *cobra.Command, args []string) {
			if err := funcs.Deploy(deploySettings); err != nil {
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

	deployCmd.PersistentFlags().StringVar(&deploySettings.DBType,
		"dbtype",
		deploySettings.DBType,
		"database type for deploy")

	return deployCmd
}
