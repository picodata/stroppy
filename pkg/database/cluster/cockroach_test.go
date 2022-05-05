package cluster

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gitlab.com/picodata/stroppy/internal/model"
	"gopkg.in/inf.v0"
)

var cockroachCluster *CockroachDatabase

func (cockroach *CockroachDatabase) TruncateTable(tableName string) error {
	sqlString := fmt.Sprintf("TRUNCATE %s", tableName)

	_, err := cockroach.pool.Exec(context.TODO(), sqlString)
	if err != nil {
		return err
	}

	return nil
}

func (cockroach *CockroachDatabase) GetAccount(Bic string, Ban string) (Account model.Account, err error) {
	var Balance int64

	dec := new(inf.Dec)

	if err := cockroach.pool.QueryRow(
		context.TODO(),
		`SELECT bic, ban, balance FROM account WHERE bic = $1 and ban = $2;`,
		Bic,
		Ban,
	).Scan(
		&Account.Bic,
		&Account.Ban,
		&Balance); err != nil {
		return model.Account{}, err
	}

	dec.SetUnscaled(Balance)
	Account.Balance = dec

	return Account, nil
}

func (cockroach *CockroachDatabase) CheckTableExist(tableName string) (exist bool, err error) {
	var name string

	sqlQuery := fmt.Sprintf("SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME='%s';", tableName)
	if err = cockroach.pool.QueryRow(context.TODO(), sqlQuery).Scan(&name); err != nil {
		return false, err
	}

	if tableName != name {
		return false, nil
	}

	return true, nil
}

func NewTestCockroachCluster(t *testing.T) {
	t.Helper()

	var err error

	cockroachURLString, err := GetEnvDataStore(Cockroach)
	if err != nil {
		t.Fatal("Get environment error:", err)
	}

	cockroachCluster, err = NewCockroachCluster(cockroachURLString, poolSize)
	if err != nil {
		t.Fatal("Cockroach cluster start fail:", err)
	}
}

func CockroachBootstrapDB(t *testing.T) {
	t.Helper()

	expectedSeed := time.Now().UnixNano()
	if err := cockroachCluster.BootstrapDB(expectedCount, int(expectedSeed)); err != nil {
		t.Errorf("TestCockroachBootstrapDB() received internal error %s, expected nil", err)
	}

	ok, err := cockroachCluster.CheckTableExist("account")
	if err != nil {
		t.Fatalf("Check table existing fail: %v", err)
	}

	if !ok {
		t.Fatalf("Table %s not existing", "account")
	}

	ok, err = cockroachCluster.CheckTableExist("transfer")
	if err != nil {
		t.Fatalf("Check table existing fail: %v", err)
	}

	if !ok {
		t.Fatalf("Table %s not existing", "transfer")
	}

	var count int
	if err := cockroachCluster.pool.QueryRow(context.TODO(), "SELECT COUNT(*) FROM account;").Scan(&count); err != nil {
		t.Errorf("TestCockroachBootstrapDB() received internal error %s, expected nil", err)
	}

	assert.Equal(t, count, 0, "Must be equal")

	if err := cockroachCluster.pool.QueryRow(context.TODO(), "SELECT COUNT(*) FROM transfer;").Scan(&count); err != nil {
		t.Errorf("TestCockroachBootstrapDB() received internal error %s, expected nil", err)
	}

	assert.Equal(t, count, 0, "Must be equal")
}

func CockroachInsertAccount(t *testing.T) {
	t.Helper()

	err := cockroachCluster.TruncateTable("account")
	if err != nil {
		t.Errorf("TestMakeAtomicTransfer() received internal error %v, but expected nil", err)
	}

	var receivedAccount model.Account

	accounts := GenerateAccounts()
	for _, expectedAccount := range accounts {
		if err := cockroachCluster.InsertAccount(expectedAccount); err != nil {
			t.Errorf("TestCockroachInsertAccount() received internal error %v, but expected nil", err)
		}

		if receivedAccount, err = cockroachCluster.GetAccount(expectedAccount.Bic, expectedAccount.Ban); err != nil {
			t.Errorf("TestCockroachInsertAccount() received internal error %v, but expected nil", err)
		}

		if expectedAccount.Ban != receivedAccount.Ban ||
			expectedAccount.Bic != receivedAccount.Bic ||
			expectedAccount.Balance.UnscaledBig().Int64() != receivedAccount.Balance.UnscaledBig().Int64() {
			t.Fail()
		}
	}
}

func CockroachMakeAtomicTransfer(t *testing.T) {
	t.Helper()

	err := cockroachCluster.TruncateTable("account")
	if err != nil {
		t.Errorf("TestMakeAtomicTransfer() received internal error %v, but expected nil", err)
	}

	var receivedAccount model.Account

	var Balance int64

	accounts := GenerateAccounts()
	dec := new(inf.Dec)

	for _, expectedAccount := range accounts {
		if err := cockroachCluster.InsertAccount(expectedAccount); err != nil {
			t.Errorf("TestCockroachInsertAccount() received internal error %v, but expected nil", err)
		}

		if receivedAccount, err = cockroachCluster.GetAccount(expectedAccount.Bic, expectedAccount.Ban); err != nil {
			t.Errorf("TestCockroachInsertAccount() received internal error %v, but expected nil", err)
		}

		if expectedAccount.Ban != receivedAccount.Ban ||
			expectedAccount.Bic != receivedAccount.Bic ||
			expectedAccount.Balance.UnscaledBig().Int64() != receivedAccount.Balance.UnscaledBig().Int64() {
			t.Fail()
		}
	}

	expectedTransfer := model.Transfer{
		ID:        model.NewTransferID(),
		Acs:       accounts,
		LockOrder: []*model.Account{},
		Amount:    rand.NewTransferAmount(),
		State:     "",
	}

	receivedTransfer := model.Transfer{
		ID:        model.TransferID{},
		Acs:       accounts,
		LockOrder: nil,
		Amount:    &inf.Dec{},
		State:     "",
	}

	if err := cockroachCluster.MakeAtomicTransfer(&expectedTransfer, uuid.UUID(rand.NewClientID())); err != nil {
		t.Errorf("TestMakeAtomicTransfer() received internal error %v, but expected nil", err)
	}

	if err := cockroachCluster.pool.QueryRow(
		context.TODO(),
		`SELECT src_bic, src_ban, dst_bic, dst_ban, amount FROM transfer WHERE transfer_id = $1;`, expectedTransfer.ID).Scan(
		&receivedTransfer.Acs[0].Bic,
		&receivedTransfer.Acs[0].Ban,
		&receivedTransfer.Acs[1].Bic,
		&receivedTransfer.Acs[1].Ban,
		&Balance); err != nil {
		t.Errorf("TestInsertAccount() received internal error %v, but expected nil", err)
	}

	dec.SetUnscaled(Balance)
	receivedTransfer.Amount = dec

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

	if receivedAccount, err = cockroachCluster.GetAccount(expectedTransfer.Acs[0].Bic, expectedTransfer.Acs[0].Ban); err != nil {
		t.Errorf("TestInsertAccount() received internal error %v, but expected nil", err)
	}

	expectedSourceBalance0 := expectedTransfer.Acs[0].Balance.UnscaledBig().Int64() - expectedTransfer.Amount.UnscaledBig().Int64()
	if receivedAccount.Balance.UnscaledBig().Int64() != expectedSourceBalance0 {
		t.Errorf("TestMakeAtomicTransfer() mismatched source balance; excepted %v  but received %v", expectedSourceBalance0, receivedAccount.Balance.UnscaledBig().Int64())
	}

	if receivedAccount, err = cockroachCluster.GetAccount(expectedTransfer.Acs[1].Bic, expectedTransfer.Acs[1].Ban); err != nil {
		t.Errorf("TestInsertAccount() received internal error %v, but expected nil", err)
	}

	expectedSourceBalance1 := expectedTransfer.Acs[1].Balance.UnscaledBig().Int64() + expectedTransfer.Amount.UnscaledBig().Int64()
	if receivedAccount.Balance.UnscaledBig().Int64() != expectedSourceBalance1 {
		t.Errorf("TestMakeAtomicTransfer() mismatched source balance; excepted %v  but received %v", expectedSourceBalance1, receivedAccount.Balance.UnscaledBig().Int64())
	}
}

func CockroachFetchAccounts(t *testing.T) {
	t.Helper()

	err := cockroachCluster.TruncateTable("account")
	if err != nil {
		t.Errorf("TestMakeAtomicTransfer() received internal error %v, but expected nil", err)
	}

	var receivedAccounts []model.Account

	accounts := GenerateAccounts()
	for _, expectedAccount := range accounts {
		if err := cockroachCluster.InsertAccount(expectedAccount); err != nil {
			t.Errorf("TestCockroachInsertAccount() received internal error %v, but expected nil", err)
		}
	}

	receivedAccounts, err = cockroachCluster.FetchAccounts()
	if err != nil {
		t.Errorf("TestCockroachInsertAccount() received internal error %v, but expected nil", err)
	}

	sort.Sort(sortAccount(accounts))
	sort.Sort(sortAccount(receivedAccounts))

	for i, account := range receivedAccounts {
		if accounts[i].Bic != account.Bic || accounts[i].Ban != account.Ban || accounts[i].Balance.Cmp(account.Balance) != 0 {
			fmt.Printf("received: Bic %s, Ban %s, Balance %s\n", account.Bic, account.Ban, account.Balance.String())
			fmt.Printf("expected: Bic %s, Ban %s, Balance %s\n", accounts[i].Bic, accounts[i].Ban, accounts[i].Balance.String())
			t.Fail()
		}
	}
}
