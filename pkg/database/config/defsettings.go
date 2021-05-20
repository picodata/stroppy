package config

import (
	"runtime"
	"time"

	llog "github.com/sirupsen/logrus"
)

const defaultCountCPU = 4

type DatabaseSettings struct {
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
	DatabaseType       string
	DBURL              string
	UseCustomTx        bool
	BanRangeMultiplier float64
}

type DeploySettings struct {
	Provider string
	Flavor   string
	Nodes    int
}

// DefaultsDeploy - заполнить параметры деплоя значениями по умолчанию.
// линтер требует указания всех полей структуры при присвоении переменной
//nolint:exhaustivestruct
func DefaultsDeploy() *DeploySettings {
	d := DeploySettings{
		Provider: "yandex",
		Flavor:   "small",
		Nodes:    3,
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
		DatabaseType:       "postgres",
		DBURL:              "",
		UseCustomTx:        false,
		BanRangeMultiplier: 1.1,
	}
}
