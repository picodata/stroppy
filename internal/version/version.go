package version

import "fmt"

type BuildVersion struct {
	Version string
	Commit  string
	Date    string
}

func (b BuildVersion) String() string {
	return fmt.Sprintf("Version: %s, Commit: %s, Build date: %s",
		b.Version, b.Commit, b.Date)
}
