/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package cluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ansel1/merry/v2"
	"github.com/google/uuid"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"gopkg.in/inf.v0"
)

// MongoDBCluster - объявление соединения к FDB и ссылки на модель данных.
type MongoDBCluster struct {
	db         *mongo.Database
	mongoModel mongoModel
	client     *mongo.Client
	sharded    bool
}

type mongoModel struct {
	accounts  *mongo.Collection
	transfers *mongo.Collection
	settings  *mongo.Collection
	checksum  *mongo.Collection
}

func (cluster *MongoDBCluster) InsertTransfer(_ *model.Transfer) error {
	return errors.New("implement me")
}

func (cluster *MongoDBCluster) DeleteTransfer(_ model.TransferId, _ uuid.UUID) error {
	return errors.New("implement me")
}

func (cluster *MongoDBCluster) SetTransferClient(clientId uuid.UUID, transferId model.TransferId) error {
	panic("implement me")
}

func (cluster *MongoDBCluster) FetchTransferClient(transferId model.TransferId) (*uuid.UUID, error) {
	panic("implement me")
}

func (cluster *MongoDBCluster) ClearTransferClient(transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cluster *MongoDBCluster) SetTransferState(state string, transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cluster *MongoDBCluster) FetchTransfer(transferId model.TransferId) (*model.Transfer, error) {
	panic("implement me")
}

func (cluster *MongoDBCluster) FetchDeadTransfers() ([]model.TransferId, error) {
	panic("implement me")
}

func (cluster *MongoDBCluster) UpdateBalance(balance *inf.Dec, bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

func (cluster *MongoDBCluster) LockAccount(transferId model.TransferId, pendingAmount *inf.Dec, bic string, ban string) (*model.Account, error) {
	panic("implement me")
}

func (cluster *MongoDBCluster) UnlockAccount(bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

// NewFoundationCluster - Создать подключение к MongoDB и создать новые коллекции, если ещё не созданы.
func NewMongoDBCluster(dbURL string, poolSize uint64, sharded bool) (*MongoDBCluster, error) {
	var clientOptions options.ClientOptions

	llog.Println(sharded)
	// задаем максимальный размер пула соединений
	clientOptions.MaxPoolSize = &poolSize

	llog.Debugln("connecting to mongodb...")
	client, err := mongo.NewClient(options.Client().ApplyURI(dbURL))
	if err != nil {
		return nil, merry.Prepend(err, "failed to create mongoDB client")
	}

	err = client.Connect(context.TODO())
	if err != nil {
		return nil, merry.Prepend(err, "failed to connect mongoDB database")
	}

	// проверяем успешность соединения
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		return nil, merry.Prepend(err, "failed to ping mongoDB database")
	}

	llog.Infoln("Connected to MongoDB: success")
	llog.Debugf("Initialized connection pool with %v connections", *clientOptions.MaxPoolSize)

	// создаем или открываем БД и коллекции - аналоги таблиц.
	db := client.Database("stroppy")
	wcMajority := writeconcern.New(writeconcern.WMajority(), writeconcern.WTimeout(10*time.Second))
	majorityCollectionOpts := options.Collection().SetWriteConcern(wcMajority)
	accounts := db.Collection("accounts", majorityCollectionOpts)
	transfers := db.Collection("transfers", majorityCollectionOpts)
	settings := db.Collection("settings")
	checksum := db.Collection("checksum")

	return &MongoDBCluster{
			db: db,
			mongoModel: mongoModel{
				accounts:  accounts,
				transfers: transfers,
				settings:  settings,
				checksum:  checksum,
			},
			client:  client,
			sharded: sharded,
		},
		nil
}

func (cluster *MongoDBCluster) addSharding() error {
	llog.Debugln("Initialize sharding...")

	enableShardingCmd := bson.D{
		{Key: "enableSharding", Value: "stroppy"},
	}

	accountShardingCmd := bson.D{
		{Key: "shardCollection", Value: "stroppy.accounts"},
		{Key: "key", Value: bson.D{{Key: "bicBan", Value: 1}}},
		{Key: "unique", Value: false},
	}

	transferShardingCmd := bson.D{
		{Key: "shardCollection", Value: "stroppy.transfers"},
		{Key: "key", Value: bson.D{{Key: "srcBic", Value: 1}}},
		{Key: "unique", Value: false},
	}

	if singleResult := cluster.client.Database("admin").RunCommand(context.TODO(), enableShardingCmd); singleResult.Err() != nil {
		return merry.Prepend(singleResult.Err(), "failed to init sharding for stroppy db")
	}

	if singleResult := cluster.client.Database("admin").RunCommand(context.TODO(), accountShardingCmd); singleResult.Err() != nil {
		return merry.Prepend(singleResult.Err(), "failed to create accounts shards")
	}

	if singleResult := cluster.client.Database("admin").RunCommand(context.TODO(), transferShardingCmd); singleResult.Err() != nil {
		return merry.Prepend(singleResult.Err(), "failed to create transfers shards")
	}

	llog.Debugln("Initialized sharding: success")
	return nil
}

// BootstrapDB - заполнить параметры настройки  и инициализировать ключ для хранения итогового баланса.
func (cluster *MongoDBCluster) BootstrapDB(count int, seed int) error {
	llog.Infof("Populating settings...")
	var insertResult *mongo.InsertOneResult
	var indexName string
	var err error

	if err = cluster.mongoModel.accounts.Drop(context.TODO()); err != nil {
		return merry.Prepend(err, "failed to clean accounts")
	}
	llog.Debugf("Cleaned accounts collection\n")

	if err = cluster.mongoModel.transfers.Drop(context.TODO()); err != nil {
		return merry.Prepend(err, "failed to clean transfers")
	}
	llog.Debugf("Cleaned transfers collection \n")

	if err = cluster.mongoModel.settings.Drop(context.TODO()); err != nil {
		return merry.Prepend(err, "failed to clean settings")
	}
	llog.Debugf("Cleaned settings collection \n")

	if err = cluster.mongoModel.checksum.Drop(context.TODO()); err != nil {
		return merry.Prepend(err, "failed to clean checksum")
	}
	llog.Debugf("Cleaned checksum collection \n")

	if insertResult, err = cluster.mongoModel.settings.InsertOne(context.TODO(), bson.D{primitive.E{Key: "count", Value: count}}, &options.InsertOneOptions{}); err != nil {
		return merry.Prepend(err, "failed to insert count value in mongodb settings")
	}

	llog.Debugf("added count in setting with id %v", insertResult)

	if insertResult, err = cluster.mongoModel.settings.InsertOne(context.TODO(), bson.D{primitive.E{Key: "seed", Value: seed}}, &options.InsertOneOptions{}); err != nil {
		return merry.Prepend(err, "failed to insert seed value in mongodb settings")
	}

	llog.Debugf("added seed in setting with id %v", insertResult)

	accountIndex := mongo.IndexModel{
		Keys:    bson.D{primitive.E{Key: "bicBan", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("accountIndex"),
	}
	// добавляем индекс для обеспечения уникальности и быстрого поиска при переводах
	if indexName, err = cluster.mongoModel.accounts.Indexes().CreateOne(context.TODO(), accountIndex); err != nil {
		return merry.Prepend(err, "failed to create account index")
	}

	llog.Debugf("Created index %v for accounts collections", indexName)

	if cluster.sharded {
		if err = cluster.addSharding(); err != nil {
			return merry.Prepend(err, "failed to enable sharding")
		}
	}

	return nil
}

// GetClusterType - получить тип DBCluster.
func (cluster *MongoDBCluster) GetClusterType() DBClusterType {
	return MongoDBClusterType
}

// FetchSettings - получить значения параметров настройки.
func (cluster *MongoDBCluster) FetchSettings() (Settings, error) {
	// добавляем явную сортировку, чтобы брать записи в порядке добавления и ходить в БД один раз
	// также убираем из вывода поле _id
	opts := options.Find().SetSort(bson.D{primitive.E{Key: "_id", Value: 1}}).SetProjection(bson.M{"_id": 0})
	cursor, err := cluster.mongoModel.settings.Find(context.TODO(), bson.D{}, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return Settings{}, ErrNoRows
		}
		return Settings{}, merry.Prepend(err, "failed to fetch settings")
	}

	defer cursor.Close(context.TODO())

	var results []map[string]int
	if err = cursor.All(context.TODO(), &results); err != nil {
		return Settings{}, merry.Prepend(err, "failed to decode cursor of  settings")
	}

	return Settings{
		Count: results[0]["count"],
		Seed:  results[1]["seed"],
	}, nil
}

// InsertAccount - сохранить новый счет.
func (cluster *MongoDBCluster) InsertAccount(acc model.Account) (err error) {
	var insertAccountResult *mongo.InsertOneResult
	var account mongo.InsertOneModel

	account.Document = bson.D{primitive.E{Key: "bicBan", Value: fmt.Sprintf("%v%v", acc.Bic, acc.Ban)}, {Key: "balance", Value: acc.Balance.UnscaledBig().Int64()}}

	if insertAccountResult, err = cluster.mongoModel.accounts.InsertOne(context.TODO(), account.Document); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return merry.Wrap(ErrDuplicateKey)
		}
		return merry.Prepend(err, "failed to insert account")
	}

	llog.Tracef("Inserted account with id %v", insertAccountResult)
	return nil
}

// FetchTotal - получить значение итогового баланса из Settings.
func (cluster *MongoDBCluster) FetchTotal() (*inf.Dec, error) {
	var balance inf.Dec
	var result map[string][]byte
	// добавляем явную сортировку, чтобы брать записи в порядке добавления и ходить в БД один раз
	// также убираем из вывода поле _id
	opts := options.FindOne().SetProjection(bson.M{"_id": 0})
	err := cluster.mongoModel.checksum.FindOne(context.TODO(), bson.D{}, opts).Decode(&result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNoRows
		}
		return nil, merry.Prepend(err, "failed to fetch total balance")
	}

	if err = balance.GobDecode(result["total"]); err != nil {
		return nil, merry.Prepend(err, "failed to decode total balance")
	}

	return &balance, nil
}

// PersistTotal - сохранить значение итогового баланса в settings.
func (cluster *MongoDBCluster) PersistTotal(total inf.Dec) error {
	var updateResult *mongo.UpdateResult
	var totalBalanceRaw []byte
	var err error

	llog.Debugf("upsent total balance %v", total)
	if totalBalanceRaw, err = total.GobEncode(); err != nil {
		return merry.Prepend(err, "failed to encode total balance for checksum")
	}

	updateOpts := options.Update().SetUpsert(true)
	filter := bson.D{}
	update := bson.D{primitive.E{Key: "$set", Value: bson.D{{Key: "total", Value: totalBalanceRaw}}}}

	updateResult, err = cluster.mongoModel.checksum.UpdateOne(context.TODO(), filter, update, updateOpts)
	if err != nil {
		return merry.Prepend(err, "failed to upsert total balance in checksum")
	}

	if updateResult.UpsertedCount == 0 && updateResult.ModifiedCount == 0 {
		return merry.Errorf("failed to upsert total balance: updated 0 documents")
	}

	llog.Debugf("inserted total balance in checksum with id %v", updateResult)
	return nil
}

// CheckBalance - рассчитать итоговый баланc.
func (cluster *MongoDBCluster) CheckBalance() (*inf.Dec, error) {
	pipe := []bson.M{
		{"$group": bson.M{
			"_id": "",
			"sum": bson.M{"$sum": "$balance"},
		}},
	}

	opts := options.Aggregate()

	opts.SetAllowDiskUse(true)

	cursor, err := cluster.mongoModel.accounts.Aggregate(context.TODO(), pipe, opts)
	if err != nil {
		return nil, err
	}

	type Result struct {
		ID      string `bson:"_id"`
		Balance int64  `bson:"sum"`
	}

	var res Result
	for cursor.Next(context.TODO()) {
		err := cursor.Decode(&res)
		if err != nil {
			return nil, err
		}
	}

	return inf.NewDec(res.Balance, 0), nil
}

func (cluster *MongoDBCluster) TopUpMoney(sessCtx mongo.SessionContext, acc model.Account, amount int64, accounts *mongo.Collection) error {
	var updatedDocument map[string]int64
	updateOpts := options.FindOneAndUpdate().SetUpsert(false).SetProjection(bson.D{
		primitive.E{Key: "_id", Value: 0},
		{Key: "bicBan", Value: 0},
	})
	filter := bson.D{primitive.E{Key: "bicBan", Value: fmt.Sprintf("%v%v", acc.Bic, acc.Ban)}}
	update := bson.D{primitive.E{Key: "$inc", Value: bson.D{{Key: "balance", Value: amount}}}}
	if err := accounts.FindOneAndUpdate(sessCtx, filter, update, updateOpts).Decode(&updatedDocument); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrNoRows
		}
		return merry.Prepend(err, "failed to update destination account balance")
	}

	if updatedDocument["balance"]-amount < 0 {
		return ErrInsufficientFunds
	}

	return nil
}

func (cluster *MongoDBCluster) WithdrawMoney(sessCtx mongo.SessionContext, acc model.Account, amount int64, accounts *mongo.Collection) error {
	var updatedDocument map[string]int64
	updateOpts := options.FindOneAndUpdate().SetUpsert(false).SetProjection(bson.D{
		primitive.E{Key: "_id", Value: 0},
		{Key: "bicBan", Value: 0},
	})
	filter := bson.D{primitive.E{Key: "bicBan", Value: fmt.Sprintf("%v%v", acc.Bic, acc.Ban)}}
	update := bson.D{primitive.E{Key: "$inc", Value: bson.D{{Key: "balance", Value: -amount}}}}
	if err := accounts.FindOneAndUpdate(sessCtx, filter, update, updateOpts).Decode(&updatedDocument); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrNoRows
		}
		return merry.Prepend(err, "failed to update destination account balance")
	}

	return nil
}

// MakeAtomicTransfer - выполнить операцию перевода и изменить балансы source и dest cчетов.
func (cluster *MongoDBCluster) MakeAtomicTransfer(transfer *model.Transfer) error {
	ctx := context.Background()
	transfers := cluster.mongoModel.transfers
	srcAccounts := cluster.mongoModel.accounts
	destAccounts := cluster.mongoModel.accounts

	transferAmount, err := transfer.Amount.GobEncode()
	if err != nil {
		return merry.Prepend(err, "failed to encode amount for transfer")
	}

	if err != nil {
		return merry.Prepend(err, "failed to insert total balance in checksum")
	}

	callback := func(sessCtx mongo.SessionContext) (interface{}, error) {
		var insertResult *mongo.InsertOneResult
		// We use algorithm as in postgres MakeAtomicTransfer() to decrease count of locks
		if transfer.Acs[0].AccountID() > transfer.Acs[1].AccountID() {

			if err = cluster.WithdrawMoney(sessCtx, transfer.Acs[0], transfer.Amount.UnscaledBig().Int64(), srcAccounts); err != nil {
				return nil, merry.Prepend(err, "failed to to withdraw money")
			}

			if err = cluster.TopUpMoney(sessCtx, transfer.Acs[1], transfer.Amount.UnscaledBig().Int64(), destAccounts); err != nil {
				return nil, merry.Prepend(err, "failed to top up money")
			}

		} else {

			if err = cluster.TopUpMoney(sessCtx, transfer.Acs[1], transfer.Amount.UnscaledBig().Int64(), destAccounts); err != nil {
				return nil, merry.Prepend(err, "failed to top up money")
			}

			if err = cluster.WithdrawMoney(sessCtx, transfer.Acs[0], transfer.Amount.UnscaledBig().Int64(), srcAccounts); err != nil {
				return nil, merry.Prepend(err, "failed to to withdraw money")
			}

		}

		docs := bson.D{
			{Key: "id", Value: transfer.Id},
			{Key: "srcBic", Value: transfer.Acs[0].Bic},
			{Key: "srcBan", Value: transfer.Acs[0].Ban},
			{Key: "destBic", Value: transfer.Acs[0].Bic},
			{Key: "destBan", Value: transfer.Acs[0].Ban},
			{Key: "LockOrder", Value: transfer.LockOrder},
			{Key: "Amount", Value: transferAmount},
			{Key: "State", Value: transfer.State},
		}
		// вставляем запись о переводе
		if insertResult, err = transfers.InsertOne(sessCtx, docs); err != nil {
			return nil, merry.Prepend(err, "failed to insert transfer")
		}
		llog.Tracef("Inserted transfer with %v and document Id %v", transfer.Id, insertResult)

		return nil, nil
	}

	session, err := cluster.client.StartSession()
	if err != nil {
		return merry.Prepend(err, "failed to start session for transaction")
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, callback)
	if err != nil {
		llog.Debugf("failed to execute transaction: %v", err)
		return merry.Wrap(err)
	}

	return nil
}

// FetchAccounts - получить список аккаунтов
func (cluster *MongoDBCluster) FetchAccounts() ([]model.Account, error) {
	return nil, nil
}

// FetchBalance - получить баланс счета по атрибутам ключа счета.
func (cluster *MongoDBCluster) FetchBalance(bic string, ban string) (*inf.Dec, *inf.Dec, error) {
	var balances, pendingAmount inf.Dec

	opts := options.Find().SetSort(bson.D{primitive.E{Key: "_id", Value: 1}}).SetProjection(bson.D{
		primitive.E{Key: "_id", Value: 0},
		{Key: "ban", Value: 0},
		{Key: "bic", Value: 0},
	})
	filter := bson.D{primitive.E{Key: "bic", Value: bic}, {Key: "ban", Value: ban}}
	cursor, err := cluster.mongoModel.accounts.Find(context.TODO(), filter, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil, ErrNoRows
		}
		return nil, nil, merry.Prepend(err, "failed to fetch checksum")
	}

	defer cursor.Close(context.TODO())

	var result []map[string][]byte
	if err = cursor.All(context.TODO(), &result); err != nil {
		return nil, nil, merry.Prepend(err, "failed to decode total balance from checksum")
	}

	if err := balances.GobDecode(result[0]["balance"]); err != nil {
		return nil, nil, merry.Prepend(err, "failed to decode  balance for account")
	}

	return &balances, &pendingAmount, nil
}

func (cluster *MongoDBCluster) StartStatisticsCollect(statInterval time.Duration) error {
	return nil
}
