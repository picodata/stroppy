package cluster

import (
	"testing"

	"gitlab.com/picodata/stroppy/internal/fixed_random_source"
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
	rand             fixed_random_source.FixedRandomSource
)

func TestNewCluster(t *testing.T) {
	NewTestMongoDBCluster(t)
	NewTestCockroachCluster(t)
}

func TestBootstrapDB(t *testing.T) {
	MongoBootstrapDB(t)
	CockroachBootstrapDB(t)
}

func TestInsertAccount(t *testing.T) {
	MongoInsertAccount(t)
	CockroachInsertAccount(t)
}

func TestMakeAtomicTransfer(t *testing.T) {
	MongoMakeAtomicTransfer(t)
	CockroachMakeAtomicTransfer(t)
}

func TestFetchAccounts(t *testing.T) {
	CockroachFetchAccounts(t)
}
