package cluster

import (
	"context"
	"errors"
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
func NewMongoDBCluster(dbURL string, poolSize uint64) (*MongoDBCluster, error) {
	var clientOptions options.ClientOptions
	// добавляем резерв соединений для перестраховки
	poolSize += reserveConnectionPool
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

	// создаем или открываем БД и коллекции - аналоги таблиц.
	db := client.Database("stroppy")
	wcMajority := writeconcern.New(writeconcern.WMajority(), writeconcern.WTimeout(1*time.Second))
	wcMajorityCollectionOpts := options.Collection().SetWriteConcern(wcMajority)
	accounts := db.Collection("accounts", wcMajorityCollectionOpts)
	transfers := db.Collection("transfers", wcMajorityCollectionOpts)
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
			client: client,
		},
		nil
}

// BootstrapDB - заполнить параметры настройки  и инициализировать ключ для хранения итогового баланса.
func (cluster *MongoDBCluster) BootstrapDB(count int, seed int) error {
	llog.Infof("Populating settings...")
	var cleanResult *mongo.DeleteResult
	var insertResult *mongo.InsertOneResult
	var indexName string
	var err error

	if cleanResult, err = cluster.mongoModel.accounts.DeleteMany(context.TODO(), bson.D{}); err != nil {
		return merry.Prepend(err, "failed to clean accounts")
	}
	llog.Debugf("drop %v documents from accounts \n", cleanResult)

	if cleanResult, err = cluster.mongoModel.transfers.DeleteMany(context.TODO(), bson.D{}); err != nil {
		return merry.Prepend(err, "failed to clean transfers")
	}
	llog.Debugf("drop %v documents from transfers \n", cleanResult)

	if cleanResult, err = cluster.mongoModel.settings.DeleteMany(context.TODO(), bson.D{}); err != nil {
		return merry.Prepend(err, "failed to clean settings")
	}
	llog.Debugf("drop %v documents from settings \n", cleanResult)

	if cleanResult, err = cluster.mongoModel.checksum.DeleteMany(context.TODO(), bson.D{}); err != nil {
		return merry.Prepend(err, "failed to clean checksum")
	}
	llog.Debugf("drop %v documents from checksum \n", cleanResult)

	if insertResult, err = cluster.mongoModel.settings.InsertOne(context.TODO(), bson.D{primitive.E{Key: "count", Value: count}}, &options.InsertOneOptions{}); err != nil {
		return merry.Prepend(err, "failed to insert count value in mongodb settings")
	}

	llog.Debugf("added count in setting with id %v", insertResult)

	if insertResult, err = cluster.mongoModel.settings.InsertOne(context.TODO(), bson.D{primitive.E{Key: "seed", Value: seed}}, &options.InsertOneOptions{}); err != nil {
		return merry.Prepend(err, "failed to insert seed value in mongodb settings")
	}

	llog.Debugf("added seed in setting with id %v", insertResult)

	accountIndex := mongo.IndexModel{
		Keys:    bson.D{primitive.E{Key: "bic", Value: 1}, {Key: "ban", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("accountIndex"),
	}
	// добавляем индекс для обеспечения уникальности и быстрого поиска при переводах
	if indexName, err = cluster.mongoModel.accounts.Indexes().CreateOne(context.TODO(), accountIndex); err != nil {
		return merry.Prepend(err, "failed to create account index")
	}

	llog.Debugf("Created index %v for accounts collections", indexName)

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

	// декодируем *inf.Dec в байтовый массив, чтобы сохранить точное значение, т.к. *inf.Dec в float не преобразуется.
	// в fdb мы сериализуем вообще весь ключ, поэтому решение видится применимым.
	balanceRaw, err := acc.Balance.GobEncode()
	if err != nil {
		return merry.Prepend(err, "failed to encode balance")
	}

	account.Document = bson.D{primitive.E{Key: "bic", Value: acc.Bic}, {Key: "ban", Value: acc.Ban}, {Key: "balance", Value: balanceRaw}}

	if insertAccountResult, err = cluster.mongoModel.accounts.InsertOne(context.TODO(), account.Document); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return merry.Wrap(ErrDuplicateKey)
		}
		return merry.Prepend(err, "failed to insert account")
	}

	llog.Infof("Inserted account with id %v", insertAccountResult)
	return nil
}

// FetchTotal - получить значение итогового баланса из Settings.
func (cluster *MongoDBCluster) FetchTotal() (*inf.Dec, error) {
	opts := options.Find().SetProjection(bson.M{"_id": 0})
	cursor, err := cluster.mongoModel.checksum.Find(context.TODO(), bson.D{}, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNoRows
		}
		return nil, merry.Prepend(err, "failed to fetch checksum")
	}

	defer cursor.Close(context.TODO())

	var result []map[string][]byte
	if err = cursor.All(context.TODO(), &result); err != nil {
		return nil, merry.Prepend(err, "failed to decode total balance from checksum")
	}

	var totalBalance inf.Dec

	if err := totalBalance.GobDecode(result[0]["total"]); err != nil {
		llog.Errorln(err)
	}

	return &totalBalance, nil
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
	totalBalance := inf.NewDec(0, 10)
	var currentBalance inf.Dec

	settings, err := cluster.FetchSettings()
	if err != nil {
		return nil, merry.Prepend(err, "failed to fetch settings for fetch accounts")
	}

	for i := 0; i < settings.Count; i = i + iterRange {
		opts := options.Find().SetSort(bson.D{primitive.E{Key: "_id", Value: 1}}).SetProjection(
			bson.D{{Key: "_id", Value: 0}, {Key: "bic", Value: 0}, {Key: "ban", Value: 0}})
		limit := int64(limitRange)
		opts.Limit = &limit
		cursor, err := cluster.mongoModel.accounts.Find(context.TODO(), bson.D{}, opts)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrNoRows
			}
			return nil, merry.Prepend(err, "failed to fetch settings")
		}

		defer cursor.Close(context.TODO())

		var results []map[string][]byte
		if err = cursor.All(context.TODO(), &results); err != nil {
			return nil, merry.Prepend(err, "failed to decode cursor of  settings")
		}

		for _, balancesMap := range results {
			if err := currentBalance.GobDecode(balancesMap["balance"]); err != nil {
				return nil, merry.Prepend(err, "failed to decode current balance")
			}
			totalBalance.Add(totalBalance, &currentBalance)
		}

	}

	return totalBalance, nil
}

// MakeAtomicTransfer - выполнить операцию перевода и изменить балансы source и dest cчетов.
func (cluster *MongoDBCluster) MakeAtomicTransfer(transfer *model.Transfer) error {
	ctx := context.Background()
	transfers := cluster.mongoModel.transfers
	srcAccounts := cluster.mongoModel.accounts
	destAccounts := cluster.mongoModel.accounts

	transferAmount, err := transfer.Amount.GobEncode()
	if err != nil {
		return merry.Prepend(err, "failed to encode amount fro transfer")
	}

	if err != nil {
		return merry.Prepend(err, "failed to insert total balance in checksum")
	}

	// Step 1: Define the callback that specifies the sequence of operations to perform inside the transaction.
	callback := func(sessCtx mongo.SessionContext) (interface{}, error) {
		var insertResult *mongo.InsertOneResult
		var srcBalance inf.Dec
		var destBalance inf.Dec
		var results []map[string][]byte

		// вставляем запись о переводе
		if insertResult, err = transfers.InsertOne(sessCtx, bson.D{
			{Key: "id", Value: transfer.Id},
			{Key: "Acs", Value: transfer.Acs},
			{Key: "LockOrder", Value: transfer.LockOrder},
			{Key: "Amount", Value: transferAmount},
			{Key: "State", Value: transfer.State},
		}); err != nil {
			return nil, merry.Prepend(err, "failed to insert transfer")
		}
		llog.Tracef("Inserted transfer with %v and document Id %v", transfer.Id, insertResult)

		// убираем лишние поля
		getOpts := options.Find().SetSort(bson.D{primitive.E{Key: "_id", Value: 1}}).SetProjection(bson.D{primitive.E{Key: "_id", Value: 0},
			{Key: "ban", Value: 0}, {Key: "bic", Value: 0}})
		// получаем баланс счета-источника
		srcAccount, err := srcAccounts.Find(context.TODO(),
			bson.D{primitive.E{Key: "bic", Value: transfer.Acs[0].Bic},
				primitive.E{Key: "ban", Value: transfer.Acs[0].Ban}},
			getOpts)
		if err != nil {
			return nil, merry.Prepend(err, "failed to get source account")
		}

		if err = srcAccount.All(context.TODO(), &results); err != nil {
			return nil, merry.Prepend(err, "failed to decode cursor for source account")
		}

		if err = srcBalance.GobDecode(results[0]["balance"]); err != nil {
			return nil, merry.Prepend(err, "failed to get source account balance")
		}

		//вычитаем сумму перевода
		srcBalance.Sub(&srcBalance, transfer.Amount)
		if srcBalance.UnscaledBig().Int64() < 0 {
			return nil, ErrInsufficientFunds
		}

		newSrcBalance, err := srcBalance.GobEncode()
		if err != nil {
			return nil, merry.Prepend(err, "failed to encode new source balance")
		}

		updateOpts := options.Update().SetUpsert(false)
		filter := bson.D{primitive.E{Key: "bic", Value: transfer.Acs[0].Bic}, {Key: "ban", Value: transfer.Acs[0].Ban}}
		update := bson.D{primitive.E{Key: "$set", Value: bson.D{{Key: "balance", Value: newSrcBalance}}}}
		if _, err := srcAccounts.UpdateOne(sessCtx, filter, update, updateOpts); err != nil {
			return nil, merry.Prepend(err, "failed to update source account balance")
		}

		// получаем баланс счета-источника
		destAccount, err := destAccounts.Find(context.TODO(),
			bson.D{primitive.E{Key: "bic", Value: transfer.Acs[1].Bic},
				primitive.E{Key: "ban", Value: transfer.Acs[1].Ban}}, getOpts)
		if err != nil {
			return nil, merry.Prepend(err, "failed to get destination account")
		}

		if err = destAccount.All(context.TODO(), &results); err != nil {
			return nil, merry.Prepend(err, "failed to decode cursor for destination account")
		}

		if err = destBalance.GobDecode(results[0]["balance"]); err != nil {
			return nil, merry.Prepend(err, "failed to get destination account balance")
		}

		destBalance.Add(&destBalance, transfer.Amount)

		newDestBalance, err := destBalance.GobEncode()
		if err != nil {
			return nil, merry.Prepend(err, "failed to encode new source balance")
		}

		update = bson.D{primitive.E{Key: "$set", Value: bson.D{{Key: "balance", Value: newDestBalance}}}}
		if _, err := destAccounts.UpdateOne(sessCtx, filter, update, updateOpts); err != nil {
			return nil, merry.Prepend(err, "failed to update destination account balance")
		}

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

	opts := options.Find().SetSort(bson.D{primitive.E{Key: "_id", Value: 1}}).SetProjection(bson.D{primitive.E{Key: "_id", Value: 0},
		{Key: "ban", Value: 0}, {Key: "bic", Value: 0}})
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
