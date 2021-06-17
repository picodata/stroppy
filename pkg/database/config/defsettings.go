package config

import (
	"runtime"
	"time"

	llog "github.com/sirupsen/logrus"
)

const defaultCountCPU = 4

const workingDirectory = "benchmark/deploy"

type BaseSettings struct {
	WorkingDirectory string
	DBType           string
}

func defaultBaseSettings() BaseSettings {
	return BaseSettings{
		WorkingDirectory: workingDirectory,
		DBType:           "postgres",
	}
}

type DatabaseSettings struct {
	BaseSettings

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

type DeploySettings struct {
	BaseSettings

	Provider string
	Flavor   string
	Nodes    int
	UseChaos bool
}

// DefaultsDeploy - заполнить параметры деплоя значениями по умолчанию.
// линтер требует указания всех полей структуры при присвоении переменной
//nolint:exhaustivestruct
func DefaultsDeploy() *DeploySettings {
	d := DeploySettings{
		Provider: "yandex",
		Flavor:   "small",
		Nodes:    3,
		UseChaos: true,

		BaseSettings: defaultBaseSettings(),
	}
	return &d
}

// Defaults - заполнить параметры для запуска тестов значениями по умолчанию
//линтер требует указания всех полей структуры при присвоении переменной
//nolint:exhaustivestruct
func Defaults() *DatabaseSettings {
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
		BaseSettings:       defaultBaseSettings(),
	}
}
