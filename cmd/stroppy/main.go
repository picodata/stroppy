package main

import (
	"runtime/debug"

	"gitlab.com/picodata/stroppy/cmd/stroppy/commands"

	"gitlab.com/picodata/stroppy/pkg/statistics"

	llog "github.com/sirupsen/logrus"
)

// The section lists vars that should be defined on build using ldflags.
// nolint gochecknoglobals
var (
	version string
	commit  string
	date    string
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			llog.Errorf("main: panic caught: '%v'\n\nstack:\n%s\n\n",
				r,
				string(debug.Stack()))
		}
	}()

	statistics.StatsInit()

	commands.UpdateBuildVersion(version, commit, date)
	commands.Execute()
}
