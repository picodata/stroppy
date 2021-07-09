package chaos

import "path/filepath"

func createScenario(name, wd string) (s scenario) {
	scenarioFileName := name + ".yaml"

	s = scenario{
		scenarioName:     name,
		scenarioFileName: scenarioFileName,

		destinationPath: filepath.Join("/home/ubuntu", scenarioFileName),
		sourcePath:      filepath.Join(wd, scenarioFileName),

		isRunning: false,
	}
	return
}

type scenario struct {
	destinationPath string
	sourcePath      string

	scenarioName     string
	scenarioFileName string

	isRunning bool
}
