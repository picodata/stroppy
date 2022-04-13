package cluster

import (
	"testing"

	"gitlab.com/picodata/stroppy/internal/model"
	"go.mongodb.org/mongo-driver/bson"
)

var mongoCluster *MongoDBCluster

type testSettings struct {
	Count int
	Seed  int
}

var (
	receivedAccount  bson.M
	receivedTransfer bson.M
	receivedAccounts []model.Account
)

func TestNewCluster(t *testing.T) {
	NewTestMongoDBCluster(t)
	NewTestCockroachCluster(t)
	NewTestPostgresCluster(t)
}

func TestBootstrapDB(t *testing.T) {
	MongoBootstrapDB(t)
	CockroachBootstrapDB(t)
	PostgresBootstrapDB(t)
}

func TestInsertAccount(t *testing.T) {
	MongoInsertAccount(t)
	CockroachInsertAccount(t)
	PostgresInsertAccount(t)
}

func TestMakeAtomicTransfer(t *testing.T) {
	MongoMakeAtomicTransfer(t)
	CockroachMakeAtomicTransfer(t)
	PostgresMakeAtomicTransfer(t)
}

func TestFetchAccounts(t *testing.T) {
	CockroachFetchAccounts(t)
	PostgresFetchAccounts(t)
}
