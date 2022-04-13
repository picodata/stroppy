package cluster

import (
	"context"
	"fmt"
	"gopkg.in/inf.v0"
	"sort"
	"testing"
	"time"

	"gitlab.com/picodata/stroppy/internal/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

var postgresCluster *PostgresCluster

func (self *PostgresCluster) CheckTableExist(tableName string) (exist bool, err error) {
	var name string
	sqlQuery := fmt.Sprintf("SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME='%s';", tableName)
	if err = self.pool.QueryRow(context.TODO(), sqlQuery).Scan(&name); err != nil {
		return false, err
	}
	if tableName != name {
		return false, nil
	}
	return true, nil
}

func (self *PostgresCluster) TruncateTable(tableName string) error {
	sqlString := fmt.Sprintf("TRUNCATE %s", tableName)
	_, err := self.pool.Exec(context.TODO(), sqlString)
	if err != nil {
		return err
	}
	return nil
}

func (self *PostgresCluster) GetAccount(Bic string, Ban string) (Account model.Account, err error) {
	var Balance int64
	dec := new(inf.Dec)

	if err := self.pool.QueryRow(
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

func NewTestPostgresCluster(t *testing.T) {
	var err error
	postgresUrlString, err := GetEnvDataStore(Postgres)
	if err != nil {
		t.Fatal("Get environment error:", err)
	}
	postgresCluster, err = NewPostgresCluster(postgresUrlString, poolSize)
	if err != nil {
		t.Fatal("Postgres cluster start fail:", err)
	}
}

func PostgresBootstrapDB(t *testing.T) {
	expectedSeed := time.Now().UnixNano()
	if err := postgresCluster.BootstrapDB(expectedCount, int(expectedSeed)); err != nil {
		t.Errorf("TestCockroachBootstrapDB() received internal error %s, expected nil", err)
	}

	ok, err := postgresCluster.CheckTableExist("account")
	if err != nil {
		t.Fatalf("Check table existing fail: %v", err)
	}
	if !ok {
		t.Fatalf("Table %s not existing", "account")
	}

	ok, err = postgresCluster.CheckTableExist("transfer")
	if err != nil {
		t.Fatalf("Check table existing fail: %v", err)
	}
	if !ok {
		t.Fatalf("Table %s not existing", "transfer")
	}

	var count int
	if err := postgresCluster.pool.QueryRow(context.TODO(), "SELECT COUNT(*) FROM account;").Scan(&count); err != nil {
		t.Errorf("TestCockroachBootstrapDB() received internal error %s, expected nil", err)
	}
	assert.Equal(t, count, 0, "Must be equal")

	if err := postgresCluster.pool.QueryRow(context.TODO(), "SELECT COUNT(*) FROM transfer;").Scan(&count); err != nil {
		t.Errorf("TestCockroachBootstrapDB() received internal error %s, expected nil", err)
	}
	assert.Equal(t, count, 0, "Must be equal")
}

func PostgresInsertAccount(t *testing.T) {
	err := postgresCluster.TruncateTable("account")
	if err != nil {
		t.Errorf("TestMakeAtomicTransfer() received internal error %v, but expected nil", err)
	}

	var receivedAccount model.Account

	accounts := GenerateAccounts()
	for _, expectedAccount := range accounts {
		if err := postgresCluster.InsertAccount(expectedAccount); err != nil {
			t.Errorf("TestCockroachInsertAccount() received internal error %v, but expected nil", err)
		}

		if receivedAccount, err = postgresCluster.GetAccount(expectedAccount.Bic, expectedAccount.Ban); err != nil {
			t.Errorf("TestCockroachInsertAccount() received internal error %v, but expected nil", err)
		}

		if expectedAccount.Ban != receivedAccount.Ban ||
			expectedAccount.Bic != receivedAccount.Bic ||
			expectedAccount.Balance.UnscaledBig().Int64() != receivedAccount.Balance.UnscaledBig().Int64() {
			t.Fail()
		}
	}
}

func PostgresMakeAtomicTransfer(t *testing.T) {
	err := postgresCluster.TruncateTable("account")
	if err != nil {
		t.Errorf("TestMakeAtomicTransfer() received internal error %v, but expected nil", err)
	}

	var receivedAccount model.Account
	accounts := GenerateAccounts()

	var Balance int64
	dec := new(inf.Dec)
	for _, expectedAccount := range accounts {
		if err := postgresCluster.InsertAccount(expectedAccount); err != nil {
			t.Errorf("TestCockroachInsertAccount() received internal error %v, but expected nil", err)
		}

		if receivedAccount, err = postgresCluster.GetAccount(expectedAccount.Bic, expectedAccount.Ban); err != nil {
			t.Errorf("TestCockroachInsertAccount() received internal error %v, but expected nil", err)
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

	if err := postgresCluster.MakeAtomicTransfer(&expectedTransfer, uuid.UUID(rand.NewClientID())); err != nil {
		t.Errorf("TestMakeAtomicTransfer() received internal error %v, but expected nil", err)
	}

	if err := postgresCluster.pool.QueryRow(
		context.TODO(),
		`SELECT src_bic, src_ban, dst_bic, dst_ban, amount FROM transfer WHERE transfer_id = $1;`, expectedTransfer.Id).Scan(
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

	if receivedAccount, err = postgresCluster.GetAccount(expectedTransfer.Acs[0].Bic, expectedTransfer.Acs[0].Ban); err != nil {
		t.Errorf("TestInsertAccount() received internal error %v, but expected nil", err)
	}

	expectedSourceBalance0 := expectedTransfer.Acs[0].Balance.UnscaledBig().Int64() - expectedTransfer.Amount.UnscaledBig().Int64()
	if receivedAccount.Balance.UnscaledBig().Int64() != expectedSourceBalance0 {
		t.Errorf("TestMakeAtomicTransfer() mismatched source balance; excepted %v  but received %v", expectedSourceBalance0, receivedAccount.Balance.UnscaledBig().Int64())
	}

	if receivedAccount, err = postgresCluster.GetAccount(expectedTransfer.Acs[1].Bic, expectedTransfer.Acs[1].Ban); err != nil {
		t.Errorf("TestInsertAccount() received internal error %v, but expected nil", err)
	}

	expectedSourceBalance1 := expectedTransfer.Acs[1].Balance.UnscaledBig().Int64() + expectedTransfer.Amount.UnscaledBig().Int64()
	if receivedAccount.Balance.UnscaledBig().Int64() != expectedSourceBalance1 {
		t.Errorf("TestMakeAtomicTransfer() mismatched source balance; excepted %v  but received %v", expectedSourceBalance1, receivedAccount.Balance.UnscaledBig().Int64())
	}
}

func PostgresFetchAccounts(t *testing.T) {
	err := postgresCluster.TruncateTable("account")
	if err != nil {
		t.Errorf("TestMakeAtomicTransfer() received internal error %v, but expected nil", err)
	}

	var receivedAccounts []model.Account

	accounts := GenerateAccounts()
	for _, expectedAccount := range accounts {
		if err := postgresCluster.InsertAccount(expectedAccount); err != nil {
			t.Errorf("TestCockroachInsertAccount() received internal error %v, but expected nil", err)
		}
	}
	receivedAccounts, err = postgresCluster.FetchAccounts()
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
