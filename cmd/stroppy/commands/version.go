/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/picodata/stroppy/internal/version"
)

var build version.BuildVersion

func UpdateBuildVersion(ver, commit, date string) {
	build = version.BuildVersion{
		Version: ver,
		Commit:  commit,
		Date:    date,
	}
}

func newVersionCommand() *cobra.Command {
	var short bool

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the agent version",
		Run: func(_ *cobra.Command, _ []string) {
			if short {
				fmt.Println(build.Version)
				return
			}
			fmt.Println(build)
		},
	}

	versionCmd.Flags().BoolVarP(
		&short, "short", "s", false,
		"print version number without any additional info")

	return versionCmd
}
