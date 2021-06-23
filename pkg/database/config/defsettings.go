package config

import (
	"runtime"
	"time"

	llog "github.com/sirupsen/logrus"
)

const defaultCountCPU = 4

const workingDirectory = "benchmark/deploy"

type Settings struct {
	WorkingDirectory string

	Local bool

	UseChaos       bool
	ChaosParameter string

	DatabaseSettings *DatabaseSettings
	DeploySettings   *DeploySettings
}

func DefaultSettings() *Settings {
	return &Settings{
		WorkingDirectory: workingDirectory,
		UseChaos:         false,
		ChaosParameter:   "container",

		Local: false,

		DeploySettings:   deployDefaults(),
		DatabaseSettings: DatabaseDefaults(),
	}
}

type DatabaseSettings struct {
	DBType   string
	LogLevel string
	Workers  int
	Count    int
	User     string
	Password string
	Seed     int64

	// long story short - enabled ZIPFian distribution means that some of bic/ban compositions
	// are used much much more often than others
	ZIPFian bool
	Oracle  bool
	Check   bool

	// TODO: add type validation in cli
	DBURL              string
	UseCustomTx        bool
	BanRangeMultiplier float64
}

// DatabaseDefaults заполняет параметры для запуска тестов значениями по умолчанию
// линтер требует указания всех полей структуры при присвоении переменной
func DatabaseDefaults() *DatabaseSettings {
	return &DatabaseSettings{
		LogLevel:           llog.InfoLevel.String(),
		Workers:            defaultCountCPU * runtime.NumCPU(),
		Count:              10000,
		User:               "",
		Password:           "",
		Check:              false,
		Seed:               time.Now().UnixNano(),
		ZIPFian:            false,
		Oracle:             false,
		DBURL:              "",
		UseCustomTx:        false,
		BanRangeMultiplier: 1.1,
		DBType:             "postgres",
	}
}

type DeploySettings struct {
	Provider string
	Flavor   string
	Nodes    int
}

// DefaultsDeploy заполняет параметры развертывания значениями по умолчанию.
// линтер требует указания всех полей структуры при присвоении переменной
func deployDefaults() *DeploySettings {
	d := DeploySettings{
		Provider: "yandex",
		Flavor:   "small",
		Nodes:    3,
	}
	return &d
}
