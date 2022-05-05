package cluster

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gitlab.com/picodata/stroppy/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/inf.v0"
)

func NewTestMongoDBCluster(t *testing.T) {
	t.Helper()

	var err error

	// пока оставляем так, чтобы потом заменить на конкретный адрес.
	sharded := false

	mongoURLString, err := GetEnvDataStore(MongoDB)
	if err != nil {
		t.Fatal("Get environment error:", err)
	}

	mongoCluster, err = NewMongoDBCluster(mongoURLString, uint64(poolSize), sharded, true)
	if err != nil {
		t.Fatal("Mongo cluster start fail:", err)
	}
}

func MongoBootstrapDB(t *testing.T) {
	t.Helper()

	var err error

	var collNames []string

	// используем значения по умолчанию.
	expectedSeed := time.Now().UnixNano()

	if err = mongoCluster.BootstrapDB(expectedCount, int(expectedSeed)); err != nil {
		t.Errorf("TestBootstrapDB() received internal error %s, expected nil", err)
	}

	if collNames, err = mongoCluster.client.Database("stroppy").ListCollectionNames(context.TODO(), bson.D{{}}); err != nil {
		t.Errorf("TestBootstrapDB() received error %s, expected list of collections", err)
	}

	// проверяем, что коллекции трансферов и контрольных сумм удалены.
	for _, coll := range collNames {
		if coll == "transfers" {
			t.Error("transfers collection found after drop. Expected collection not found")
		} else if coll == "checksum" {
			t.Error("checksum collection found after drop. Expected collection not found")
		}
	}

	// проверяем, что коллекция счетов есть, но пустая.
	count, err := mongoCluster.mongoModel.accounts.CountDocuments(context.TODO(), bson.D{{}})
	if count != 0 {
		t.Errorf("Expected 0 documents in accounts, but received %v", err)
	}

	// проверяем корректность заполнения настроек
	opts := options.Find().SetSort(bson.D{primitive.E{Key: "_id", Value: 1}}).SetProjection(bson.M{"_id": 0})

	cursor, err := mongoCluster.mongoModel.settings.Find(context.TODO(), bson.D{}, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			t.Error("Expected count and seed values, but nothing documents found")
		}

		t.Errorf("TestBootstrapDB() received error %s, but expected cursor from settings collection", err)
	}

	defer cursor.Close(context.TODO())

	var results []map[string]int

	if err = cursor.All(context.TODO(), &results); err != nil {
		t.Errorf("TestBootstrapDB() received error %s, but expected result of decode cursor from settings collection", err)
	}

	if results[0]["count"] != expectedCount {
		t.Errorf("Expected count %v, received %v", expectedCount, results[0]["count"])
	}

	if results[1]["seed"] != int(expectedSeed) {
		t.Errorf("Expected count %v, received %v", expectedSeed, results[0]["seed"])
	}

	indexes, err := mongoCluster.mongoModel.accounts.Indexes().List(context.TODO())
	if err != nil {
		t.Errorf("Expected list of indexes, received %v", err)
	}

	var indexList []bson.M

	expectedIndex := bson.M{}

	if err = indexes.All(context.TODO(), &indexList); err != nil {
		t.Errorf("Expected opening cursor, received %v", err)
	}

	for _, index := range indexList {
		if index["name"] == "accountIndex" {
			expectedIndex = index
		}
	}

	if len(expectedIndex) == 0 {
		t.Error("Expected index accountIndex, but index not found")
	}
}

func MongoInsertAccount(t *testing.T) {
	t.Helper()

	for i := 0; i < 2; i++ {
		rand.Init(expectedCount, int(time.Now().UnixNano()), defaultBanRangeMultiplier)
		bic, ban := rand.NewBicAndBan()
		balance := rand.NewStartBalance()
		expectedAccount := model.Account{
			Bic:           bic,
			Ban:           ban,
			Balance:       balance,
			PendingAmount: &inf.Dec{},
			Found:         false,
		}

		expectedABicBan := fmt.Sprintf("%v%v", expectedAccount.Bic, expectedAccount.Ban)

		if err := mongoCluster.InsertAccount(expectedAccount); err != nil {
			t.Errorf("TestInsertAccount() received internal error %v, but expected nil", err)
		}

		// убираем лишние поля
		getOpts := options.FindOne().SetSort(bson.D{primitive.E{Key: "_id", Value: 1}}).SetProjection(bson.D{
			primitive.E{Key: "_id", Value: 0},
		})

		// получаем баланс счета-источника
		if err := mongoCluster.mongoModel.accounts.FindOne(context.TODO(), bson.D{
			primitive.E{Key: "bicBan", Value: expectedABicBan},
		}, getOpts).Decode(&receivedAccount); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				t.Error("TestInsertAccount() expected 1 document, but received 0 document")
			}

			t.Errorf("TestInsertAccount() expected 1 document, but received error %v", err)
		}

		if receivedAccount["bicBan"] != expectedABicBan {
			t.Errorf("TestInsertAccount() expected %v , but received %v", expectedABicBan, receivedAccount["bicBan"])
		}

		if receivedAccount["balance"] != expectedAccount.Balance.UnscaledBig().Int64() {
			t.Errorf("TestInsertAccount() expected %v , but received %v", expectedABicBan, receivedAccount["bicBan"])
		}

		receivedAccounts = append(receivedAccounts, model.Account{
			Bic:             expectedAccount.Bic,
			Ban:             expectedAccount.Ban,
			Balance:         expectedAccount.Balance,
			PendingAmount:   &inf.Dec{},
			PendingTransfer: [16]byte{},
			Found:           false,
		})
	}
}

func MongoMakeAtomicTransfer(t *testing.T) {
	t.Helper()

	expectedTransfer := model.Transfer{
		ID:        model.NewTransferID(),
		Acs:       receivedAccounts,
		LockOrder: []*model.Account{},
		Amount:    rand.NewTransferAmount(),
		State:     "",
	}

	if err := mongoCluster.MakeAtomicTransfer(&expectedTransfer, uuid.UUID(rand.NewClientID())); err != nil {
		t.Errorf("TestMakeAtomicTransfer() received internal error %v, but expected nil", err)
	}

	getOpts := options.FindOne().SetSort(bson.D{primitive.E{Key: "_id", Value: 1}}).SetProjection(bson.D{
		primitive.E{Key: "_id", Value: 0},
	})

	if err := mongoCluster.mongoModel.transfers.FindOne(context.TODO(), bson.D{primitive.E{Key: "id", Value: expectedTransfer.ID}}, getOpts).Decode(&receivedTransfer); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			t.Error("TestMakeAtomicTransfer() expected 1 document of transfer, but received 0 document")
		}

		t.Errorf("TestMakeAtomicTransfer() expected 1 document of transfer, but received error %v", err)
	}

	if receivedTransfer["srcBic"] != expectedTransfer.Acs[0].Bic {
		t.Errorf("TestMakeAtomicTransfer() expected source Bic %v , but received %v", expectedTransfer.Acs[0].Bic, receivedTransfer["srcBin"])
	}

	if receivedTransfer["srcBan"] != expectedTransfer.Acs[0].Ban {
		t.Errorf("TestMakeAtomicTransfer() expected source Ban %v , but received %v", expectedTransfer.Acs[0].Ban, receivedTransfer["srcBan"])
	}

	if receivedTransfer["destBic"] != expectedTransfer.Acs[1].Bic {
		t.Errorf("TestMakeAtomicTransfer() expected destination Bic %v , but received %v", expectedTransfer.Acs[1].Bic, receivedTransfer["destBin"])
	}

	if receivedTransfer["destBan"] != expectedTransfer.Acs[1].Ban {
		t.Errorf("TestMakeAtomicTransfer() expected destination Ban %v , but received %v", expectedTransfer.Acs[1].Ban, receivedTransfer["destBan"])
	}

	if receivedTransfer["Amount"] != expectedTransfer.Amount.UnscaledBig().Int64() {
		t.Errorf("TestMakeAtomicTransfer() expected transfer amount %v , but received %v", expectedTransfer.Amount.UnscaledBig().Int64(), receivedTransfer["Amount"])
	}

	// проверяем, что баланс изменился у обоих счетов
	getOpts = options.FindOne().SetSort(bson.D{primitive.E{Key: "_id", Value: 1}}).SetProjection(bson.D{primitive.E{Key: "_id", Value: 0}})
	if err := mongoCluster.mongoModel.accounts.FindOne(context.TODO(), bson.D{
		primitive.E{Key: "bicBan", Value: fmt.Sprintf("%v%v", expectedTransfer.Acs[0].Bic, expectedTransfer.Acs[0].Ban)},
	}, getOpts).Decode(&receivedAccount); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			t.Error("TestMakeAtomicTransfer() expected source account, but received 0 document")
		}

		t.Errorf("TestMakeAtomicTransfer() expected source account, but received error %v", err)
	}

	expectedSourceBalance := expectedTransfer.Acs[0].Balance.UnscaledBig().Int64() - expectedTransfer.Amount.UnscaledBig().Int64()
	if receivedAccount["balance"] != expectedSourceBalance {
		t.Errorf("TestMakeAtomicTransfer() mismatched source balance; excepted %v  but received %v", expectedSourceBalance, receivedAccount["balance"])
	}

	if err := mongoCluster.mongoModel.accounts.FindOne(context.TODO(), bson.D{
		primitive.E{Key: "bicBan", Value: fmt.Sprintf("%v%v", expectedTransfer.Acs[1].Bic, expectedTransfer.Acs[1].Ban)},
	}, getOpts).Decode(&receivedAccount); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			t.Error("TestMakeAtomicTransfer() expected destination account, but received 0 document")
		}

		t.Errorf("TestMakeAtomicTransfer() expected destination account, but received error %v", err)
	}

	expectedDestBalance := expectedTransfer.Acs[1].Balance.UnscaledBig().Int64() + expectedTransfer.Amount.UnscaledBig().Int64()
	if receivedAccount["balance"] != expectedDestBalance {
		t.Errorf("TestMakeAtomicTransfer() mismatched dest balance; excepted %v  but received error %v", expectedDestBalance, receivedAccount["balance"])
	}
}
