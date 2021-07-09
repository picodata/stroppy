package chaos

import llog "github.com/sirupsen/logrus"

func createDummyChaos() (c Controller) {
	c = &dummyChaos{}
	return
}

type dummyChaos struct{}

func (_ *dummyChaos) Deploy() (_ error) {
	llog.Infof("Dummy chaos successfully deployed\n")
	return
}

func (_ *dummyChaos) ExecuteCommand(scenarioName string) (_ error) {
	llog.Infof("dummy chaos successfully execute `%s` scenario\n", scenarioName)
	return
}

func (_ *dummyChaos) Stop() {
	llog.Infof("dummy chaos successfully stopped\n")
}
