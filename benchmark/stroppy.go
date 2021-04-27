package main

import (
	"os"
	"runtime"
	"time"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type DatabaseSettings struct {
	log_level string
	workers   int
	count     int
	user      string
	password  string
	seed      int64
	// long story short - enabled zipfian distribution means that some of bic/ban compositions
	// are used much much more often than others
	zipfian bool
	oracle  bool
	check   bool
	// TODO: add type validation in cli
	databaseType       string
	dbURL              string
	useCustomTx        bool
	banRangeMultiplier float64
}

type DeploySettings struct {
	provider string
	flavor   string
	nodes    int
}

const defaultCountCPU = 4

// DefaultsDeploy - заполнить параметры деплоя значениями по умолчанию.
// линтер требует указания всех полей структуры при присвоении переменной
//nolint:exhaustivestruct
func DefaultsDeploy() DeploySettings {
	d := DeploySettings{}
	d.provider = "yandex"
	d.flavor = "small"
	d.nodes = 3
	return d
}

// Defaults - заполнить параметры для запуска тестов значениями по умолчанию
//линтер требует указания всех полей структуры при присвоении переменной
//nolint:exhaustivestruct
func Defaults() DatabaseSettings {
	s := DatabaseSettings{}
	s.log_level = llog.InfoLevel.String()
	s.workers = defaultCountCPU * runtime.NumCPU()
	s.count = 10000
	s.user = ""
	s.password = ""
	s.check = false
	s.seed = time.Now().UnixNano()
	s.zipfian = false
	s.oracle = false
	s.databaseType = "postgres"
	s.dbURL = ""
	s.useCustomTx = false
	s.banRangeMultiplier = 1.1
	return s
}

func main() {

	llog.SetOutput(os.Stdout)

	formatter := new(llog.TextFormatter)
	// Stackoverflow wisdom
	formatter.TimestampFormat = "Jan _2 15:04:05.000"
	formatter.FullTimestamp = true
	formatter.ForceColors = true
	llog.SetFormatter(formatter)
	settings := Defaults()
	deploySettings := DefaultsDeploy()

	var rootCmd = &cobra.Command{
		Use:   "lightest [pop|pay|deploy]",
		Short: "lightest - a sample LWT application implementing an account ledger",
		Long: `
This program models an automatic banking system.  It implements 3 model
workloads, for populating the database with accounts, making transfers, and
checking correctness. It collects client-side metrics for latency and
bandwidth along the way.`,
		Version: "0.9",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if l, err := llog.ParseLevel(settings.log_level); err != nil {
				return merry.Wrap(err)
			} else {
				llog.SetLevel(l)
			}
			if settings.workers > settings.count && settings.count > 0 {
				settings.workers = settings.count
			}
			StatsSetTotal(settings.count)
			return nil
		},
	}
	rootCmd.PersistentFlags().StringVarP(&settings.log_level,
		"log-level", "v",
		settings.log_level,
		"Log level (trace, debug, info, warn, error, fatal, panic")
	rootCmd.PersistentFlags().StringVarP(&settings.user,
		"user", "u",
		settings.user,
		"Database user")
	rootCmd.PersistentFlags().StringVarP(&settings.password,
		"password", "p",
		settings.password,
		"Database password")
	rootCmd.PersistentFlags().StringVarP(&settings.databaseType,
		"database", "d",
		settings.databaseType,
		"Database type, postgreSQL if not set.")
	rootCmd.PersistentFlags().StringVar(&settings.dbURL,
		"url",
		settings.dbURL,
		"Connection string, required flag")
	rootCmd.PersistentFlags().Float64VarP(&settings.banRangeMultiplier,
		"banRangeMultiplier", "r",
		settings.banRangeMultiplier,
		`
ban range multiplier (next brm) is a number that defines
the ratio of BAN (Bank Identifier Number) per BIC (Bank Identifier Code). 
The number of generated BICs is approximately equal to the square root of 'count'. 
The count of BANs is defined by the following formula: Nban = (Nbic * brm)/square(count). 
If Nban * Nbic > count we generate more (BIC, BAN) combinations 
than we saved during DB population process (that is achieved if brm > 1).
The recommended range of brm is from 1.01 to 1.1. 
The default value of banRangeMultipluer is 1.1.`)
	rootCmd.PersistentFlags().IntVarP(&settings.workers,
		"workers", "w",
		settings.workers,
		"Number of workers, 4 * NumCPU if not set.")

	var popCmd = &cobra.Command{
		Use:     "pop",
		Aliases: []string{"populate"},
		Short:   "Create and populate the accounts database",
		Example: "./lightest populate -n 100000000",

		Run: func(cmd *cobra.Command, args []string) {
			if err := populate(&settings); err != nil {
				llog.Fatalf("%v", err)
			}

			balance, err := check(&settings, nil)
			if err != nil {
				llog.Fatalf("%v", err)
			}
			llog.Infof("Total balance: %v", balance)
		},
	}
	popCmd.PersistentFlags().IntVarP(&settings.count,
		"count", "n",
		settings.count,
		"Number of accounts to create")
	// заполняем все поля, для неиспользуемых указвыаем nil, согласно требованиям линтера
	//nolint:gofumpt
	var deployCmd = &cobra.Command{
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
			if err := deploy(deploySettings); err != nil {
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

	deployCmd.PersistentFlags().StringVar(&deploySettings.provider,
		"cloud",
		deploySettings.provider,
		"name of cloud provider")
	deployCmd.PersistentFlags().StringVar(&deploySettings.flavor,
		"flavor",
		deploySettings.flavor,
		"name of cluster configuration from templates.yml")
	deployCmd.PersistentFlags().IntVar(&deploySettings.nodes,
		"nodes",
		deploySettings.nodes,
		"count nodes of cluster")

	var payCmd = &cobra.Command{
		Use:     "pay",
		Aliases: []string{"transfer"},
		Short:   "Run the payments workload",
		Run: func(cmd *cobra.Command, args []string) {
			sum, err := check(&settings, nil)
			if err != nil {
				llog.Fatalf("%v", err)
			}

			llog.Infof("Initial balance: %v", sum)

			if err := pay(&settings); err != nil {
				llog.Fatalf("%v", err)
			}
			if settings.check {
				balance, err := check(&settings, sum)
				if err != nil {
					llog.Fatalf("%v", err)
				}
				llog.Infof("Final balance: %v", balance)
			}
		},
	}
	payCmd.PersistentFlags().IntVarP(&settings.count,
		"count", "n", settings.count,
		"Number of transfers to make")
	payCmd.PersistentFlags().BoolVarP(&settings.zipfian,
		"zipfian", "z", settings.zipfian,
		"Use zipfian distribution for payments")
	payCmd.PersistentFlags().BoolVarP(&settings.oracle,
		"oracle", "o", settings.oracle,
		"Check all payments against the built-in oracle.")
	payCmd.PersistentFlags().BoolVarP(&settings.check,
		"check", "", settings.check,
		"Check the final balance to match the original one (set to false if benchmarking).")
	payCmd.PersistentFlags().BoolVarP(&settings.useCustomTx,
		"tx", "t", settings.useCustomTx,
		"Use custom implementation of atomic transactions (workaround for dbs without built-in ACID transactions).")
	rootCmd.AddCommand(popCmd, payCmd, deployCmd)
	StatsInit()
	_ = rootCmd.Execute()
}
