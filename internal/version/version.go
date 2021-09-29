/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

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
