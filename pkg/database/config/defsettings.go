package config

import (
	"runtime"
	"time"

	"gitlab.com/picodata/stroppy/pkg/database/cluster"

	llog "github.com/sirupsen/logrus"
)

const (
	defaultCountCPU = 4

	workingDirectory = "benchmark/deploy"

	chaosParameterFdb = "fdb-pod-kill-first,fdb-pod-kill-second"
	chaosParameterPg  = "pg-pod-kill-first,pg-pod-kill-second"
)

type Settings struct {
	WorkingDirectory string
	LogLevel         string

	Local bool

	UseChaos       bool
	ChaosParameter string

	TestSettings *TestSettings

	DatabaseSettings   *DatabaseSettings
	DeploymentSettings *DeploymentSettings

	DestroyOnExit bool
}

func DefaultSettings() (s *Settings) {
	s = &Settings{
		WorkingDirectory: workingDirectory,
		UseChaos:         false,

		Local: false,

		DeploymentSettings: deployDefaults(),
		DatabaseSettings:   DatabaseDefaults(),

		TestSettings: TestDefaults(),

		LogLevel:      llog.InfoLevel.String(),
		DestroyOnExit: false,
	}

	switch s.DatabaseSettings.DBType {
	case cluster.Postgres:
		s.ChaosParameter = chaosParameterPg
	case cluster.Foundation:
		s.ChaosParameter = chaosParameterFdb
	default:
		s.ChaosParameter = ""
	}
	return
}

type TestSettings struct {
	KubernetesMasterAddress string
	UseCloudStroppy         bool
	RunAsPod                bool
}

func TestDefaults() *TestSettings {
	return &TestSettings{
		KubernetesMasterAddress: "",
		UseCloudStroppy:         false,
		RunAsPod:                false,
	}
}

type DatabaseSettings struct {
	DBType   string
	Workers  int
	Count    int
	User     string
	Password string
	Seed     int64

	// long story short - enabled Zipfian distribution means that some of bic/ban compositions
	// are used much much more often than others
	Zipfian bool
	Oracle  bool
	Check   bool

	// TODO: add type validation in cli
	DBURL              string
	UseCustomTx        bool
	BanRangeMultiplier float64
	StatInterval       time.Duration
	AddPool            int
}

// DatabaseDefaults заполняет параметры для запуска тестов значениями по умолчанию
// линтер требует указания всех полей структуры при присвоении переменной
func DatabaseDefaults() *DatabaseSettings {
	return &DatabaseSettings{
		Workers:            defaultCountCPU * runtime.NumCPU(),
		Count:              10000,
		User:               "",
		Password:           "",
		Check:              false,
		Seed:               time.Now().UnixNano(),
		Zipfian:            false,
		Oracle:             false,
		DBURL:              "",
		UseCustomTx:        false,
		BanRangeMultiplier: 1.1,
		DBType:             cluster.Postgres,
		StatInterval:       10,
		AddPool:            0,
	}
}

type DeploymentSettings struct {
	Provider string
	Flavor   string
	Nodes    int
}

// DefaultsDeploy заполняет параметры развертывания значениями по умолчанию.
// линтер требует указания всех полей структуры при присвоении переменной
func deployDefaults() *DeploymentSettings {
	d := DeploymentSettings{
		Provider: "yandex",
		Flavor:   "small",
		Nodes:    3,
	}
	return &d
}
