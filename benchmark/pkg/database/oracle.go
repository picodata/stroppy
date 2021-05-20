package database

import (
	"sync"

	"gitlab.com/picodata/stroppy/benchmark/internal/model"

	llog "github.com/sirupsen/logrus"
	"gopkg.in/inf.v0"
)

type PredictableCluster interface {
	// Return list of all present accounts
	FetchAccounts() ([]model.Account, error)
	// Provide balance and pending amount for an account for Oracle.
	FetchBalance(bic string, ban string) (balance *inf.Dec, pendingAmount *inf.Dec, err error)
}

type TrackingAccount struct {
	bic        string
	ban        string
	balance    *inf.Dec
	transferId model.TransferId
}

func (acc *TrackingAccount) setTransfer(transferId model.TransferId) {
	if acc.transferId != model.NilUuid && acc.transferId != transferId {
		llog.Fatalf("setTransfer() on %v:%v: current transfer id %v, setting %v",
			acc.bic, acc.ban, acc.transferId, transferId)
	}
	acc.transferId = transferId
}

func (acc *TrackingAccount) clearTransfer(transferId model.TransferId) {
	if acc.transferId != model.NilUuid && acc.transferId != transferId {
		llog.Fatalf("clearTransfer() on %v:%v: current transfer id %v, clearing %v",
			acc.bic, acc.ban, acc.transferId, transferId)
	}
	acc.transferId = model.NilUuid
}

func (acc *TrackingAccount) BeginDebit(transferId model.TransferId, amount *inf.Dec) {
	acc.setTransfer(transferId)
}

func (acc *TrackingAccount) CompleteDebit(transferId model.TransferId, amount *inf.Dec) {
	acc.clearTransfer(transferId)
	acc.balance.Sub(acc.balance, amount)
}

func (acc *TrackingAccount) BeginCredit(transferId model.TransferId, amount *inf.Dec) {
	acc.setTransfer(transferId)
}

func (acc *TrackingAccount) CompleteCredit(transferId model.TransferId, amount *inf.Dec) {
	acc.clearTransfer(transferId)
	acc.balance.Add(acc.balance, amount)
}

type Oracle struct {
	acs       map[string]*TrackingAccount
	transfers map[model.TransferId]bool
	mux       sync.Mutex
}

func (o *Oracle) Init(cluster PredictableCluster) {
	llog.Infof("Oracle enabled, loading account balances")
	o.acs = make(map[string]*TrackingAccount)
	o.transfers = make(map[model.TransferId]bool)
	accounts, err := cluster.FetchAccounts()
	if err != nil {
		llog.Fatalf("%v", err)
	}
	for _, acc := range accounts {
		o.acs[acc.Bic+acc.Ban] = &TrackingAccount{
			bic:     acc.Bic,
			ban:     acc.Ban,
			balance: acc.Balance,
		}
	}
}

func (o *Oracle) lookupAccounts(acs []model.Account) (*TrackingAccount, *TrackingAccount) {
	from, from_found := o.acs[acs[0].Bic+acs[0].Ban]
	to, to_found := o.acs[acs[1].Bic+acs[1].Ban]
	if (!from_found || !to_found) && acs[0].Found && acs[1].Found {
		llog.Fatalf("One of the accounts is found, while it's missing")
	}
	return from, to
}

func (o *Oracle) BeginTransfer(transferId model.TransferId, acs []model.Account, amount *inf.Dec) {
	o.mux.Lock()
	defer o.mux.Unlock()
	if _, exists := o.transfers[transferId]; exists {
		llog.Tracef("Double execution of the same transfer %v", transferId)
		// Have processed this transfer already
		return
	}
	if from, to := o.lookupAccounts(acs); from != nil && to != nil && amount.Cmp(from.balance) <= 0 {
		from.BeginDebit(transferId, amount)
		to.BeginCredit(transferId, amount)
	}
}

func (o *Oracle) CompleteTransfer(transferId model.TransferId, acs []model.Account, amount *inf.Dec) {
	o.mux.Lock()
	defer o.mux.Unlock()
	if _, exists := o.transfers[transferId]; exists {
		// Have processed this transfer already
		return
	}
	o.transfers[transferId] = true
	if from, to := o.lookupAccounts(acs); from != nil && to != nil && amount.Cmp(from.balance) <= 0 {
		from.CompleteDebit(transferId, amount)
		to.CompleteCredit(transferId, amount)
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
