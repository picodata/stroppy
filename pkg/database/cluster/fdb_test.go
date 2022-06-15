package cluster

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ansel1/merry"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/google/uuid"

	"gitlab.com/picodata/stroppy/internal/model"
	"gopkg.in/inf.v0"
)

var fdbCluster *FDBCluster

func (cluster *FDBCluster) CheckTableExist(tableName string) (exist bool, err error) {
	_, err = cluster.pool.ReadTransact(func(tr fdb.ReadTransaction) (interface{}, error) {
		exist, err = directory.Exists(tr, []string{tableName})
		return nil, nil
	})
	return exist, err
}

func (cluster *FDBCluster) TruncateTable() error {
	_, err := cluster.pool.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.ClearRange(cluster.model.accounts)
		tx.ClearRange(cluster.model.transfers)
		return nil, nil
	})
	return err
}

func (cluster *FDBCluster) GetAccount(Bic string, Ban string) (account model.Account, err error) {
	var value accountValue
	_, err = cluster.pool.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		key := cluster.model.accounts.Pack(tuple.Tuple{Bic, Ban})
		valueSrc, err := tx.Get(key).Get()
		if err != nil {
			return nil, merry.Wrap(err)
		}
		if len(valueSrc) == 0 {
			return nil, ErrNoRows
		}
		err = json.Unmarshal(valueSrc, &value)
		if err != nil {
			return nil, merry.Wrap(err)
		}
		return nil, nil
	})

	if err != nil {
		return model.Account{}, merry.Wrap(err)
	}

	account.Bic = Bic
	account.Ban = Ban
	account.Balance = value.Balance

	return account, nil
}

func (cluster *FDBCluster) GetTransfer(expectedTransfer model.Transfer) (transfer model.Transfer, err error) {
	var value transferValue
	_, err = cluster.pool.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		key := cluster.model.transfers.Pack(
			tuple.Tuple{tuple.UUID(expectedTransfer.Id),
				expectedTransfer.Acs[0].Bic,
				expectedTransfer.Acs[0].Ban,
				expectedTransfer.Acs[1].Bic,
				expectedTransfer.Acs[1].Ban,
			})
		valueSrc, err := tx.Get(key).Get()
		if err != nil {
			return nil, merry.Wrap(err)
		}
		if len(valueSrc) == 0 {
			return nil, ErrNoRows
		}
		err = json.Unmarshal(valueSrc, &value)
		if err != nil {
			return nil, merry.Wrap(err)
		}
		return nil, nil
	})

	if err != nil {
		return model.Transfer{}, merry.Wrap(err)
	}

	transfer = model.Transfer{
		Id:        expectedTransfer.Id,
		Acs:       expectedTransfer.Acs,
		LockOrder: nil,
		Amount:    value.Amount,
		State:     "",
	}
	return transfer, nil
}

func (cluster *FDBCluster) isEmpty(tableName string) bool {
	var sw directory.DirectorySubspace
	switch tableName {
	case "accounts":
		sw = cluster.model.accounts
	case "transfers":
		sw = cluster.model.transfers
	default:
		return false
	}
	var r []fdb.KeyValue
	_, _ = cluster.pool.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		r = tx.GetRange(sw, fdb.RangeOptions{Limit: 0, Mode: fdb.StreamingModeWantAll, Reverse: false}).GetSliceOrPanic()
		return nil, nil
	})
	return len(r) == 0
}

func NewTestFDBCluster(t *testing.T) {
	var err error
	fdbUrlString, err := GetEnvDataStore(Foundation)
	if err != nil {
		t.Fatal("Get environment error:", err)
	}
	fdbCluster, err = NewFoundationCluster(fdbUrlString)
	if err != nil {
		t.Fatal("FDB cluster start fail:", err)
	}
}

func FDBBootstrapDB(t *testing.T) {
	expectedSeed := time.Now().UnixNano()
	if err := fdbCluster.BootstrapDB(expectedCount, int(expectedSeed)); err != nil {
		t.Errorf("TestFDBBootstrapDB() received internal error %s, expected nil", err)
	}

	ok, err := fdbCluster.CheckTableExist("accounts")
	if err != nil {
		t.Fatalf("Check table existing fail: %v", err)
	}
	if !ok {
		t.Fatalf("Table %s not existing", "accounts")
	}

	ok, err = fdbCluster.CheckTableExist("transfers")
	if err != nil {
		t.Fatalf("Check table existing fail: %v", err)
	}
	if !ok {
		t.Fatalf("Table %s not existing", "transfers")
	}

	if !fdbCluster.isEmpty("accounts") {
		t.Fail()
	}
	if !fdbCluster.isEmpty("transfers") {
		t.Fail()
	}
}

func FDBInsertAccount(t *testing.T) {
	err := fdbCluster.TruncateTable()
	if err != nil {
		t.Errorf("TestFDBInsertAccount() received internal error %v, but expected nil", err)
	}

	var receivedAccount model.Account

	accounts := GenerateAccounts()
	for _, expectedAccount := range accounts {
		if err := fdbCluster.InsertAccount(expectedAccount); err != nil {
			t.Errorf("TestFDBInsertAccount() received internal error %v, but expected nil", err)
		}

		if receivedAccount, err = fdbCluster.GetAccount(expectedAccount.Bic, expectedAccount.Ban); err != nil {
			t.Errorf("TestFDBInsertAccount() received internal error %v, but expected nil", err)
		}

		if expectedAccount.Ban != receivedAccount.Ban ||
			expectedAccount.Bic != receivedAccount.Bic ||
			expectedAccount.Balance.UnscaledBig().Int64() != receivedAccount.Balance.UnscaledBig().Int64() {
			t.Fail()
		}
	}
}

func FDBMakeAtomicTransfer(t *testing.T) {
	err := fdbCluster.TruncateTable()
	if err != nil {
		t.Errorf("TestFDBMakeAtomicTransfer() received internal error %v, but expected nil", err)
	}

	var receivedAccount model.Account
	accounts := GenerateAccounts()

	for _, expectedAccount := range accounts {
		if err := fdbCluster.InsertAccount(expectedAccount); err != nil {
			t.Errorf("TestFDBMakeAtomicTransfer() received internal error %v, but expected nil", err)
		}

		if receivedAccount, err = fdbCluster.GetAccount(expectedAccount.Bic, expectedAccount.Ban); err != nil {
			t.Errorf("TestFDBMakeAtomicTransfer() received internal error %v, but expected nil", err)
		}

		if expectedAccount.Ban != receivedAccount.Ban ||
			expectedAccount.Bic != receivedAccount.Bic ||
			expectedAccount.Balance.UnscaledBig().Int64() != receivedAccount.Balance.UnscaledBig().Int64() {
			t.Fail()
		}
	}

	expectedTransfer := model.Transfer{
		Id:        model.NewTransferId(),
		Acs:       accounts,
		LockOrder: []*model.Account{},
		Amount:    rand.NewTransferAmount(),
		State:     "",
	}

	receivedTransfer := model.Transfer{
		Id:        model.TransferId{},
		Acs:       accounts,
		LockOrder: nil,
		Amount:    &inf.Dec{},
		State:     "",
	}

	if err := fdbCluster.MakeAtomicTransfer(&expectedTransfer, uuid.UUID(rand.NewClientID())); err != nil {
		t.Errorf("TestMakeAtomicTransfer() received internal error %v, but expected nil", err)
	}
	receivedTransfer, err = fdbCluster.GetTransfer(expectedTransfer)

	if receivedTransfer.Acs[0].Bic != expectedTransfer.Acs[0].Bic {
		t.Errorf("TestMakeAtomicTransfer() expected source Bic %v , but received %v", expectedTransfer.Acs[0].Bic, receivedTransfer.Acs[0].Bic)
	}
	if receivedTransfer.Acs[0].Ban != expectedTransfer.Acs[0].Ban {
		t.Errorf("TestMakeAtomicTransfer() expected source Bic %v , but received %v", expectedTransfer.Acs[0].Ban, receivedTransfer.Acs[0].Ban)
	}

	if receivedTransfer.Acs[1].Bic != expectedTransfer.Acs[1].Bic {
		t.Errorf("TestMakeAtomicTransfer() expected source Bic %v , but received %v", expectedTransfer.Acs[1].Bic, receivedTransfer.Acs[1].Bic)
	}
	if receivedTransfer.Acs[1].Ban != expectedTransfer.Acs[1].Ban {
		t.Errorf("TestMakeAtomicTransfer() expected source Bic %v , but received %v", expectedTransfer.Acs[1].Ban, receivedTransfer.Acs[1].Ban)
	}

	if receivedAccount, err = fdbCluster.GetAccount(expectedTransfer.Acs[0].Bic, expectedTransfer.Acs[0].Ban); err != nil {
		t.Errorf("TestInsertAccount() received internal error %v, but expected nil", err)
	}

	var res inf.Dec
	expectedSourceBalance0 := res.Sub(expectedTransfer.Acs[0].Balance, expectedTransfer.Amount).UnscaledBig().Int64()
	if receivedAccount.Balance.UnscaledBig().Int64() != expectedSourceBalance0 {
		t.Errorf("TestMakeAtomicTransfer() mismatched source balance; excepted %d  but received %d", expectedSourceBalance0, receivedAccount.Balance.UnscaledBig().Int64())
	}

	if receivedAccount, err = fdbCluster.GetAccount(expectedTransfer.Acs[1].Bic, expectedTransfer.Acs[1].Ban); err != nil {
		t.Errorf("TestInsertAccount() received internal error %v, but expected nil", err)
	}

	expectedSourceBalance1 := res.Add(expectedTransfer.Acs[1].Balance, expectedTransfer.Amount).UnscaledBig().Int64()
	if receivedAccount.Balance.UnscaledBig().Int64() != expectedSourceBalance1 {
		t.Errorf("TestMakeAtomicTransfer() mismatched source balance; excepted %d  but received %d", expectedSourceBalance1, receivedAccount.Balance.UnscaledBig().Int64())
	}
}
