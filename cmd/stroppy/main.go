package main

import (
	"os"

	"gitlab.com/picodata/stroppy/cmd/stroppy/commands"
	"gitlab.com/picodata/stroppy/pkg/statistics"

	llog "github.com/sirupsen/logrus"
)

func main() {
	llog.SetOutput(os.Stdout)

	formatter := new(llog.TextFormatter)
	// Stackoverflow wisdom
	formatter.TimestampFormat = "Jan _2 15:04:05.000"
	formatter.FullTimestamp = true
	formatter.ForceColors = true
	llog.SetFormatter(formatter)

	statistics.StatsInit()
	commands.Execute()
}
