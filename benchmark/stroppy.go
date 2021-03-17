package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	llog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type Settings struct {
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
	databaseType string
	dbURL        string
	useCustomTx  bool
}

func Defaults() Settings {
	s := Settings{}
	s.log_level = llog.InfoLevel.String()
	s.workers = 4 * runtime.NumCPU()
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

	var rootCmd = &cobra.Command{
		Use:   "lightest [pop|pay]",
		Short: "lightest - a sample LWT application implementing an account ledger",
		Long: `
This program models an automatic banking system.  It implements 3 model
workloads, for populating the database with accounts, making transfers, and
checking correctness. It collects client-side metrics for latency and
bandwidth along the way.`,
		Version: "0.9",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if l, err := llog.ParseLevel(settings.log_level); err != nil {
				return err
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
		"Cassandra user")
	rootCmd.PersistentFlags().StringVarP(&settings.password,
		"password", "p",
		settings.password,
		"Cassandra password")
	rootCmd.PersistentFlags().StringVarP(&settings.databaseType,
		"database", "d",
		settings.databaseType,
		"Database type, postgreSQL if not set.")
	rootCmd.PersistentFlags().StringVar(&settings.dbURL,
		"url",
		settings.dbURL,
		"Connection string, required flag")
	if err := rootCmd.MarkPersistentFlagRequired("url"); err != nil {
		panic(fmt.Errorf("failed to mark flag \"url\" required, err: %s", err))
	}
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
	rootCmd.AddCommand(popCmd, payCmd)
	StatsInit()
	_ = rootCmd.Execute()
}