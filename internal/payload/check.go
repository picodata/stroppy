/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package payload

import (
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gopkg.in/inf.v0"
)

func (p *BasePayload) Check(prev *inf.Dec) (sum *inf.Dec, err error) {
	// Only persist the balance if it is not persisted yet
	// Only calculate the balance if it's necessary to persist
	// it, or it is necessary for a Check (prev != nil)
	persistBalance := false

	if prev == nil {
		sum, err = p.Cluster.FetchTotal()
		if err != nil {
			if err != cluster.ErrNoRows {
				llog.Fatalf("Failed to fetch the stored total: %v", err)
			}
			sum = nil
			persistBalance = true
		}
	}

	if sum == nil {
		llog.Infof("Calculating the total balance...")
		if sum, err = p.Cluster.CheckBalance(); err != nil {
			llog.Fatalf("Failed to calculate the total: %v", err)
		}
	}

	if prev != nil {
		if prev.Cmp(sum) != 0 {
			llog.Fatalf("Check balance mismatch:\nbefore: %v\nafter:  %v", prev, sum)
		}
	}

	if persistBalance {
		// Do not overwrite the total balance if it is already persisted.
		llog.Infof("Persisting the total balance...")
		if err := p.Cluster.PersistTotal(*sum); err != nil {
			llog.Fatalf("Failed to persist total balance: error %v, sum: %v", err, sum)
		}
	}

	return
}
