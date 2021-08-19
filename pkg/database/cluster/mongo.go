package cluster

import (
	"context"
	"errors"
	"time"

	"github.com/ansel1/merry/v2"
	"github.com/google/uuid"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/internal/model"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/inf.v0"
)

// MongoDBCluster - объявление соединения к FDB и ссылки на модель данных.
type MongoDBCluster struct {
	db         *mongo.Database
	mongoModel mongoModel
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
	accounts := db.Collection("accounts")
	transfers := db.Collection("transfers")
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
		},
		nil
}

// BootstrapDB - заполнить параметры настройки  и инициализировать ключ для хранения итогового баланса.
func (cluster *MongoDBCluster) BootstrapDB(count int, seed int) error {
	llog.Infof("Populating settings...")

	return nil
}

// GetClusterType - получить тип DBCluster.
func (cluster *MongoDBCluster) GetClusterType() DBClusterType {
	return MongoDBClusterType
}

// FetchSettings - получить значения параметров настройки.
func (cluster *MongoDBCluster) FetchSettings() (Settings, error) {

	return Settings{}, nil
}

// InsertAccount - сохранить новый счет.
func (cluster *MongoDBCluster) InsertAccount(acc model.Account) error {

	return nil
}

// FetchTotal - получить значение итогового баланса из Settings.
func (cluster *MongoDBCluster) FetchTotal() (*inf.Dec, error) {
	return nil, nil
}

// PersistTotal - сохранить значение итогового баланса в settings.
func (cluster *MongoDBCluster) PersistTotal(total inf.Dec) error {

	return nil
}

// CheckBalance - рассчитать итоговый баланc.
func (cluster *MongoDBCluster) CheckBalance() (*inf.Dec, error) {

	return nil, nil
}

// MakeAtomicTransfer - выполнить операцию перевода и изменить балансы source и dest cчетов.
func (cluster *MongoDBCluster) MakeAtomicTransfer(transfer *model.Transfer) error {

	return nil
}

// FetchAccounts - получить список аккаунтов
func (cluster *MongoDBCluster) FetchAccounts() ([]model.Account, error) {
	return nil, nil
}

// FetchBalance - получить баланс счета по атрибутам ключа счета.
func (cluster *MongoDBCluster) FetchBalance(bic string, ban string) (*inf.Dec, *inf.Dec, error) {
	return nil, nil, nil
}

func (cluster *MongoDBCluster) StartStatisticsCollect(statInterval time.Duration) error {

	return nil
}
