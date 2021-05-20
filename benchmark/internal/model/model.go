package model

import (
	"fmt"
	"time"

	"gitlab.com/picodata/stroppy/benchmark/internal/fixed_random_source"

	"github.com/google/uuid"
	llog "github.com/sirupsen/logrus"
	"gopkg.in/inf.v0"
)

type TransferId = uuid.UUID

func NewTransferId() TransferId {
	return uuid.New()
}

var NilUuid = uuid.UUID{}

// Some random bank account
type Account struct {
	Bic             string
	Ban             string
	Balance         *inf.Dec
	PendingAmount   *inf.Dec
	PendingTransfer TransferId
	Found           bool
}

func (acc Account) AccountID() string {
	return acc.Bic + acc.Ban
}

// A record with data about money transfer from acc1 -> acc2
type Transfer struct {
	Id        TransferId
	Acs       []Account
	LockOrder []*Account
	Amount    *inf.Dec
	State     string
}

func (t *Transfer) InitAccounts() {
	if t.Amount == nil {
		llog.Fatalf("[%v] Found transfer with nil amount", t.Id)
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

func (t *Transfer) InitRandomTransfer(randSource *fixed_random_source.FixedRandomSource, zipfian bool) {
	t.Amount = randSource.NewTransferAmount()
	t.Acs = make([]Account, 2)
	if zipfian {
		t.Acs[0].Bic, t.Acs[0].Ban = randSource.HotBicAndBan()
		t.Acs[1].Bic, t.Acs[1].Ban = randSource.HotBicAndBan(t.Acs[0].Bic, t.Acs[0].Ban)
	} else {
		t.Acs[0].Bic, t.Acs[0].Ban = randSource.BicAndBan()
		t.Acs[1].Bic, t.Acs[1].Ban = randSource.BicAndBan(t.Acs[0].Bic, t.Acs[0].Ban)
	}
	t.Id = NewTransferId()
	t.State = "new"
	t.InitAccounts()
}

func (t *Transfer) InitEmptyTransfer(id TransferId) {
	t.Id = id
	t.Acs = make([]Account, 2)
}

func (t *Transfer) String() string {
	return fmt.Sprintf("transfer from %v:%v (%v) to %v:%v (%v) - %v",
		t.Acs[0].Bic, t.Acs[0].Ban, t.Acs[0].Balance,
		t.Acs[1].Bic, t.Acs[1].Ban, t.Acs[1].Balance,
		t.Amount)
}

// A account history item
type HistoryItem struct {
	ID            uuid.UUID
	TransferID    TransferId
	AccountBic    string
	AccountBan    string
	OldBalance    *inf.Dec
	NewBalance    *inf.Dec
	OperationTime time.Time
}

func NewHistoryItem(
	tranfserID uuid.UUID,
	bic string,
	ban string,
	oldBalance *inf.Dec,
	newBalance *inf.Dec,
	operationTime time.Time,
) HistoryItem {
	historyID, err := uuid.NewUUID()
	if err != nil {
		// TODO: don't panic
		panic(err)
	}

	return HistoryItem{
		ID:            historyID,
		TransferID:    tranfserID,
		AccountBic:    bic,
		AccountBan:    ban,
		OldBalance:    oldBalance,
		NewBalance:    newBalance,
		OperationTime: operationTime,
	}
}
