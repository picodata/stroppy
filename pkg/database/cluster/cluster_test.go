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

const (
	poolSize                  = 128
	mongoDBUrl                     = "mongodb://127.0.0.1:30001,127.0.0.1:30002,127.0.0.1:30003/stroppy"
	expectedCount             = 10000
	defaultBanRangeMultiplier = 1.1
)

func TestNewCluster(t *testing.T) {
	NewTestMongoDBCluster(t)
}

func TestBootstrapDB(t *testing.T) {
	MongoBootstrapDB(t)
}

func TestInsertAccount(t *testing.T) {
	MongoInsertAccount(t)
}

func TestMakeAtomicTransfer(t *testing.T) {
	MongoMakeAtomicTransfer(t)
}
