/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package database

import (
	"sync"

	"gitlab.com/picodata/stroppy/internal/model"

	llog "github.com/sirupsen/logrus"
	"gopkg.in/inf.v0"
)

type PredictableCluster interface {
	// FetchAccounts returns list of all present accounts
	FetchAccounts() ([]model.Account, error)

	// FetchBalance returns balance and pending amount for an account for Oracle.
	FetchBalance(bic string, ban string) (balance *inf.Dec, pendingAmount *inf.Dec, err error)
}

type TrackingAccount struct {
	bic        string
	ban        string
	balance    *inf.Dec
	transferID model.TransferID
}

func (acc *TrackingAccount) setTransfer(transferID model.TransferID) {
	if acc.transferID != model.NilUUID && acc.transferID != transferID {
		llog.Fatalf("setTransfer() on %v:%v: current transfer id %v, setting %v",
			acc.bic, acc.ban, acc.transferID, transferID)
	}

	acc.transferID = transferID
}

func (acc *TrackingAccount) clearTransfer(transferID model.TransferID) {
	if acc.transferID != model.NilUUID && acc.transferID != transferID {
		llog.Fatalf("clearTransfer() on %v:%v: current transfer id %v, clearing %v",
			acc.bic, acc.ban, acc.transferID, transferID)
	}

	acc.transferID = model.NilUUID
}

func (acc *TrackingAccount) BeginDebit(transferID model.TransferID, amount *inf.Dec) {
	acc.setTransfer(transferID)
}

func (acc *TrackingAccount) CompleteDebit(transferID model.TransferID, amount *inf.Dec) {
	acc.clearTransfer(transferID)
	acc.balance.Sub(acc.balance, amount)
}

func (acc *TrackingAccount) BeginCredit(transferID model.TransferID, amount *inf.Dec) {
	acc.setTransfer(transferID)
}

func (acc *TrackingAccount) CompleteCredit(transferID model.TransferID, amount *inf.Dec) {
	acc.clearTransfer(transferID)
	acc.balance.Add(acc.balance, amount)
}

type Oracle struct {
	acs       map[string]*TrackingAccount
	transfers map[model.TransferID]bool
	mux       sync.Mutex
}

func (o *Oracle) Init(cluster PredictableCluster) {
	llog.Infof("Oracle enabled, loading account balances")

	o.acs = make(map[string]*TrackingAccount)
	o.transfers = make(map[model.TransferID]bool)

	accounts, err := cluster.FetchAccounts()
	if err != nil {
		llog.Fatalf("%v", err)
	}

	for _, acc := range accounts {
		o.acs[acc.Bic+acc.Ban] = &TrackingAccount{
			bic:        acc.Bic,
			ban:        acc.Ban,
			balance:    acc.Balance,
			transferID: model.TransferID{},
		}
	}
}

func (o *Oracle) lookupAccounts(acs []model.Account) (from, to *TrackingAccount) {
	var fromFound, toFound bool
	from, fromFound = o.acs[acs[0].Bic+acs[0].Ban]
	to, toFound = o.acs[acs[1].Bic+acs[1].Ban]

	if (!fromFound || !toFound) && acs[0].Found && acs[1].Found {
		llog.Fatalf("One of the accounts is found, while it's missing")
	}

	return from, to
}

func (o *Oracle) BeginTransfer(transferID model.TransferID, acs []model.Account, amount *inf.Dec) {
	o.mux.Lock()
	defer o.mux.Unlock()

	if _, exists := o.transfers[transferID]; exists {
		llog.Tracef("Double execution of the same transfer %v", transferID)
		// Have processed this transfer already
		return
	}

	if from, to := o.lookupAccounts(acs); from != nil && to != nil && amount.Cmp(from.balance) <= 0 {
		from.BeginDebit(transferID, amount)
		to.BeginCredit(transferID, amount)
	}
}

func (o *Oracle) CompleteTransfer(transferID model.TransferID, acs []model.Account, amount *inf.Dec) {
	o.mux.Lock()
	defer o.mux.Unlock()

	if _, exists := o.transfers[transferID]; exists {
		// Have processed this transfer already
		return
	}

	o.transfers[transferID] = true
	if from, to := o.lookupAccounts(acs); from != nil && to != nil && amount.Cmp(from.balance) <= 0 {
		from.CompleteDebit(transferID, amount)
		to.CompleteCredit(transferID, amount)
	}
}

func (o *Oracle) FindBrokenAccounts(cluster PredictableCluster) {
	for _, acc := range o.acs {
		balance, _, err := cluster.FetchBalance(acc.bic, acc.ban)
		if err != nil {
			llog.Errorf("failed to fetch balance with bic %v, ban %v from cluster", acc.bic, acc.ban)

			continue
		}

		if balance.Cmp(acc.balance) != 0 {
			llog.Errorf("%v:%v balance is %v should be %v", acc.bic, acc.ban, balance, acc.balance)
		}
	}
}
