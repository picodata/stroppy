/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package model

import (
	"fmt"

	"gitlab.com/picodata/stroppy/internal/fixedrandomsource"

	"github.com/google/uuid"
	llog "github.com/sirupsen/logrus"
	"gopkg.in/inf.v0"
)

type TransferID = uuid.UUID

func NewTransferID() TransferID {
	return uuid.New()
}

var NilUUID = uuid.UUID{}

// Account Some random bank account.
type Account struct {
	Bic             string
	Ban             string
	Balance         *inf.Dec
	PendingAmount   *inf.Dec
	PendingTransfer TransferID
	Found           bool
}

func (acc Account) AccountID() string {
	return acc.Bic + acc.Ban
}

// Transfer A record with data about money transfer from acc1 -> acc2.
type Transfer struct {
	ID        TransferID
	Acs       []Account
	LockOrder []*Account
	Amount    *inf.Dec
	State     string
}

func (t *Transfer) InitAccounts() {
	if t.Amount == nil {
		llog.Fatalf("[%v] Found transfer with nil amount", t.ID)
	}

	acs := t.Acs
	t.LockOrder = make([]*Account, 2)
	// Always lock accounts in lexicographical order to avoid livelocks
	if acs[1].Bic > acs[0].Bic ||
		acs[1].Bic == acs[0].Bic &&
			acs[1].Ban > acs[0].Ban {
		t.LockOrder[0] = &t.Acs[0]
		t.LockOrder[1] = &t.Acs[1]
	} else {
		t.LockOrder[0] = &t.Acs[1]
		t.LockOrder[1] = &t.Acs[0]
	}
	// Use pending amount as a flag to avoid double transfer on recover
	acs[0].PendingAmount = new(inf.Dec).Neg(t.Amount)
	acs[1].PendingAmount = t.Amount
}

func (t *Transfer) InitRandomTransfer(randSource *fixedrandomsource.FixedRandomSource, zipfian bool) {
	t.Amount = randSource.NewTransferAmount()
	t.Acs = make([]Account, 2)

	if zipfian {
		t.Acs[0].Bic, t.Acs[0].Ban = randSource.HotBicAndBan()
		t.Acs[1].Bic, t.Acs[1].Ban = randSource.HotBicAndBan(t.Acs[0].Bic, t.Acs[0].Ban)
	} else {
		t.Acs[0].Bic, t.Acs[0].Ban = randSource.BicAndBan()
		t.Acs[1].Bic, t.Acs[1].Ban = randSource.BicAndBan(t.Acs[0].Bic, t.Acs[0].Ban)
	}

	t.ID = NewTransferID()
	t.State = "new"
	t.InitAccounts()
}

func (t *Transfer) InitEmptyTransfer(id TransferID) {
	t.ID = id
	t.Acs = make([]Account, 2)
}

func (t *Transfer) String() string {
	return fmt.Sprintf("transfer from %v:%v (%v) to %v:%v (%v) - %v",
		t.Acs[0].Bic, t.Acs[0].Ban, t.Acs[0].Balance,
		t.Acs[1].Bic, t.Acs[1].Ban, t.Acs[1].Balance,
		t.Amount)
}
