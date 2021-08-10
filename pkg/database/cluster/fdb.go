package cluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"gitlab.com/picodata/stroppy/internal/model"

	"github.com/ansel1/merry"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	llog "github.com/sirupsen/logrus"
	"gopkg.in/inf.v0"
)

const versionAPI = 620

const iterRange = 100000

const limitRange = 100001

// FDBCluster - объявление соединения к FDB и ссылки на модель данных.
type FDBCluster struct {
	pool  fdb.Database
	model modelFDB
}

func (cluster *FDBCluster) InsertTransfer(_ *model.Transfer) error {
	return errors.New("implement me")
}

func (cluster *FDBCluster) DeleteTransfer(_ model.TransferId, _ uuid.UUID) error {
	return errors.New("implement me")
}

func (cluster *FDBCluster) SetTransferClient(clientId uuid.UUID, transferId model.TransferId) error {
	panic("implement me")
}

func (cluster *FDBCluster) FetchTransferClient(transferId model.TransferId) (*uuid.UUID, error) {
	panic("implement me")
}

func (cluster *FDBCluster) ClearTransferClient(transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cluster *FDBCluster) SetTransferState(state string, transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cluster *FDBCluster) FetchTransfer(transferId model.TransferId) (*model.Transfer, error) {
	panic("implement me")
}

func (cluster *FDBCluster) FetchDeadTransfers() ([]model.TransferId, error) {
	panic("implement me")
}

func (cluster *FDBCluster) UpdateBalance(balance *inf.Dec, bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

func (cluster *FDBCluster) LockAccount(transferId model.TransferId, pendingAmount *inf.Dec, bic string, ban string) (*model.Account, error) {
	panic("implement me")
}

func (cluster *FDBCluster) UnlockAccount(bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

// modelFDB - объявление модели данных.
type modelFDB struct {
	accounts  directory.DirectorySubspace
	transfers directory.DirectorySubspace
	settings  directory.DirectorySubspace
	checksum  directory.DirectorySubspace
}

// transferValue - объявление атрибутов перевода.
type transferValue struct {
	Amount *inf.Dec `json:"Amount"`
}

// accountValue - объявление атрибутов счета.
type accountValue struct {
	Balance *inf.Dec `json:"Balance"`
}

// NewFoundationCluster - Создать подключение к FDB и создать новые DirectorySubspace, если ещё не созданы.
func NewFoundationCluster(dbURL string) (*FDBCluster, error) {
	llog.Infof("Establishing connection to FDB on %v", dbURL)
	poolConfig := dbURL

	err := fdb.APIVersion(versionAPI)
	if err != nil {
		return nil, merry.Prepend(err, "failed to check version FDB API")
	}

	var FDBPool fdb.Database
	if FDBPool, err = fdb.OpenDatabase(poolConfig); err != nil {
		return nil, merry.Prepend(err, "fdb.cluster file open")
	}
	llog.Infof("Creating or opening the subspaces... \n")
	// создаем или открываем Directory - часть Directory cо своими метаданынми

	accounts, err := directory.CreateOrOpen(FDBPool, []string{"accounts"}, nil)
	if err != nil {
		return nil, merry.Prepend(err, "failed to create accounts directory")
	}

	transfers, err := directory.CreateOrOpen(FDBPool, []string{"transfers"}, nil)
	if err != nil {
		return nil, merry.Prepend(err, "failed to create transfers directory")
	}

	settings, err := directory.CreateOrOpen(FDBPool, []string{"settings"}, nil)
	if err != nil {
		return nil, merry.Prepend(err, "failed to create settings directory")
	}

	checkSum, err := directory.CreateOrOpen(FDBPool, []string{"checksum"}, nil)
	if err != nil {
		return nil, merry.Prepend(err, "failed to create checksum directory")
	}

	return &FDBCluster{
		pool: FDBPool,
		model: modelFDB{
			accounts:  accounts,
			transfers: transfers,
			settings:  settings,
			checksum:  checkSum,
		},
	}, nil
}

// BootstrapDB - заполнить параметры настройки  и инициализировать ключ для хранения итогового баланса.
func (cluster *FDBCluster) BootstrapDB(count int, seed int) error {
	llog.Infof("Populating settings...")
	_, err := cluster.pool.Transact(func(tx fdb.Transaction) (interface{}, error) {
		// очищаем cпейсы перед началом загрузки счетов, BootstrapDB вызывается на этом этапе
		tx.ClearRange(cluster.model.accounts)
		tx.ClearRange(cluster.model.checksum)
		tx.ClearRange(cluster.model.settings)
		tx.ClearRange(cluster.model.transfers)
		countKey := cluster.model.settings.Pack(tuple.Tuple{"count"})
		seedKey := cluster.model.settings.Pack(tuple.Tuple{"seed"})
		checkSumTotalKey := cluster.model.checksum.Pack(tuple.Tuple{"total"})
		tx.Set(countKey, []byte(strconv.Itoa(count)))
		tx.Set(seedKey, []byte(strconv.Itoa(seed)))
		// добавляем пустое значение для checksum, чтобы инициировать ключ
		tx.Set(checkSumTotalKey, []byte{})
		return nil, nil
	})
	if err != nil {
		return merry.Prepend(err, "failed to populate settings")
	}
	return nil
}

// GetClusterType - получить тип DBCluster.
func (cluster *FDBCluster) GetClusterType() DBClusterType {
	return FDBClusterType
}

// FetchSettings - получить значения параметров настройки.
func (cluster *FDBCluster) FetchSettings() (Settings, error) {
	var clusterSettings Settings
	data, err := cluster.pool.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		var fetchCount, fetchSeed int
		countKey := cluster.model.settings.Pack(tuple.Tuple{"count"})
		seedKey := cluster.model.settings.Pack(tuple.Tuple{"seed"})
		count, err := tx.Get(countKey).Get()
		if err != nil {
			return nil, merry.Prepend(err, "failed to get count from Settings")
		}
		seed, err := tx.Get(seedKey).Get()
		if err != nil {
			return nil, merry.Prepend(err, "failed to get seed from Settings")
		}
		err = json.Unmarshal(count, &fetchCount)
		if err != nil {
			return nil, merry.Prepend(err, "failed to deserialize count from Settings")
		}
		err = json.Unmarshal(seed, &fetchSeed)
		if err != nil {
			return nil, merry.Prepend(err, "failed to deserialize seed from Settings")
		}
		return Settings{
			Count: fetchCount,
			Seed:  fetchSeed,
		}, nil
	})
	if err != nil {
		// не удается вернуть nil, возникает ошибка
		return clusterSettings, merry.Prepend(err, "failed to fetch from Settings")
	}
	clusterSettings, ok := data.(Settings)
	if !ok {
		return clusterSettings, merry.Errorf("this data type ClusterSettings is not supported")
	}
	return clusterSettings, nil
}

// InsertAccount - сохранить новый счет.
func (cluster *FDBCluster) InsertAccount(acc model.Account) error {
	_, err := cluster.pool.Transact(func(tx fdb.Transaction) (interface{}, error) {
		keyAccount := cluster.getAccountKey(acc)
		checkUniq, err := tx.Get(keyAccount).Get()
		if checkUniq != nil {
			return nil, ErrDuplicateKey
		}
		if err != nil {
			return nil, merry.Prepend(err, "failed to check account for existence")
		}
		var valueAccount accountValue
		valueAccount.Balance = acc.Balance
		valueAccountSet, err := serializeValue(valueAccount)
		if err != nil {
			return nil, merry.Prepend(err, "failed to serialize account value for insert")
		}
		tx.Set(keyAccount, valueAccountSet)
		// оставляем просто err, т.к. обработчик определен после транзакции
		return nil, nil
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateKey) {
			return ErrDuplicateKey
		}
		return merry.Prepend(err, "failed to insert account")
	}

	return nil
}

// FetchTotal - получить значение итогового баланса из Settings.
func (cluster *FDBCluster) FetchTotal() (*inf.Dec, error) {
	data, err := cluster.pool.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		var checkSumValue inf.Dec
		checkSumKey := cluster.model.checksum.Pack(tuple.Tuple{"total"})
		checkSumValueRaw, err := tx.Get(checkSumKey).Get()
		// обработчик определен после транзакции
		if err != nil {
			return nil, merry.Wrap(err)
		}
		checkSumValue, err = deserializeCheckSum(checkSumValueRaw)
		// обработчик определен после транзакции
		if err != nil {
			return nil, err
		}
		return checkSumValue, nil
	})
	if err != nil {
		if !errors.Is(err, ErrNoRows) {
			return nil, merry.Prepend(err, "failed to fetch total balance")
		}
		return nil, ErrNoRows
	}
	valueCheckSum, ok := data.(inf.Dec)
	if !ok {
		return nil, merry.Errorf("this data type of CheckSum is not supported")
	}
	return &valueCheckSum, nil
}

// PersistTotal - сохранить значение итогового баланса в settings.
func (cluster *FDBCluster) PersistTotal(total inf.Dec) error {
	_, err := cluster.pool.Transact(func(tx fdb.Transaction) (interface{}, error) {
		totalKey := cluster.model.checksum.Pack(tuple.Tuple{"total"})
		totalValue, err := serializeCheckSum(total)
		if err != nil {
			return totalValue, merry.Prepend(err, "failed to serialize total balance value")
		}
		tx.Set(totalKey, totalValue)
		return nil, nil
	})
	if err != nil {
		return merry.Prepend(err, "failed to save total balance in CheckSum")
	}
	return nil
}

// CheckBalance - рассчитать итоговый баланc.
func (cluster *FDBCluster) CheckBalance() (*inf.Dec, error) {
	totalBalance := inf.NewDec(0, 10)

	var accountsKeyValuesArray []fdb.KeyValue

	var data interface{}

	beginKey, endKey := cluster.model.accounts.FDBRangeKeys()

	settings, err := cluster.FetchSettings()
	if err != nil {
		return nil, merry.Prepend(err, "failed to fetch settings for fetch accounts")
	}
	/*
		Если кол-во счетов больше iterRange, получаем счета пачками по iterRange и в цикле складывает балансы,
		по всему массиву, кроме последнего элемента. На последней итерации складываем балансы всего массива.
		Если кол-во меньше, то получаем весь массив счетов и в цикле складываем балансы сразу по всему массиву
	*/
	if settings.Count >= iterRange {
		for i := 0; i < settings.Count; i = i + iterRange {
			data, err = cluster.pool.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
				keyValueArray, err := tx.GetRange(fdb.KeyRange{
					Begin: beginKey,
					End:   endKey,
				},
					fdb.RangeOptions{Limit: limitRange, Mode: fdb.StreamingModeWantAll, Reverse: false}).GetSliceWithError()
				if err != nil {
					return nil, merry.Wrap(err)
				}

				return keyValueArray, nil
			})

			if err != nil {
				return nil, merry.Prepend(err, "failed to fetch accounts")
			}

			accountsKeyValues, ok := data.([]fdb.KeyValue)
			if !ok {
				return nil, merry.Errorf("this type data of fdb.KeyValue is not supported")
			}

			if len(accountsKeyValues) == limitRange {
				accountsKeyValuesArray = accountsKeyValues[:len(accountsKeyValues)-1]
			} else {
				accountsKeyValuesArray = accountsKeyValues
			}

			for _, accountKeyValue := range accountsKeyValuesArray {

				var fetchAccountValue accountValue

				fetchAccountValue, err = deserializeAccountValue(accountKeyValue.Value)
				if err != nil {
					return nil, err
				}

				totalBalance = totalBalance.Add(totalBalance, fetchAccountValue.Balance)
			}

			beginKey = accountsKeyValues[len(accountsKeyValues)-1].Key

		}
	} else {

		fetchResult, err := cluster.FetchAccounts()
		if err != nil {
			return nil, merry.Prepend(err, "failed to get Accounts array")
		}
		for i := range fetchResult {
			fetchAccount := fetchResult[i]
			totalBalance = totalBalance.Add(totalBalance, fetchAccount.Balance)
		}

	}

	return totalBalance, nil
}

// MakeAtomicTransfer - выполнить операцию перевода и изменить балансы source и dest cчетов.
func (cluster *FDBCluster) MakeAtomicTransfer(transfer *model.Transfer) error {
	_, err := cluster.pool.Transact(func(tx fdb.Transaction) (interface{}, error) {
		err := cluster.setTransfer(tx, transfer)
		if err != nil {
			return nil, err
		}
		srcAccount := model.Account{
			Bic:             transfer.Acs[0].Bic,
			Ban:             transfer.Acs[0].Ban,
			Balance:         &inf.Dec{},
			PendingAmount:   &inf.Dec{},
			PendingTransfer: [16]byte{},
			Found:           false,
		}
		sourceAccountKey := cluster.getAccountKey(srcAccount)
		sourceAccountValue, err := getAccountValue(tx, sourceAccountKey)
		if err != nil {
			if errors.Is(err, ErrNoRows) {
				return nil, ErrNoRows
			}
			return nil, merry.Prepend(err, "failed to get source account")
		}
		sourceAccountValue.Balance.Sub(sourceAccountValue.Balance, transfer.Amount)
		if sourceAccountValue.Balance.UnscaledBig().Int64() < 0 {
			return nil, ErrInsufficientFunds
		}
		setSourceAccountValue, err := serializeValue(sourceAccountValue)
		if err != nil {
			return nil, merry.Prepend(err, "failed to serialize source account")
		}
		tx.Set(sourceAccountKey, setSourceAccountValue)
		destAccount := model.Account{
			Bic:             transfer.Acs[1].Bic,
			Ban:             transfer.Acs[1].Ban,
			Balance:         &inf.Dec{},
			PendingAmount:   &inf.Dec{},
			PendingTransfer: [16]byte{},
			Found:           false,
		}
		destAccountKey := cluster.getAccountKey(destAccount)
		destAccountValue, err := getAccountValue(tx, destAccountKey)
		if err != nil {
			if errors.Is(err, ErrNoRows) {
				return nil, ErrNoRows
			}
			return nil, merry.Prepend(err, "failed to get destination account")
		}
		destAccountValue.Balance.Add(destAccountValue.Balance, transfer.Amount)
		setDestAccountValue, err := serializeValue(destAccountValue)
		if err != nil {
			return nil, merry.Prepend(err, "failed to serialize destination account")
		}
		tx.Set(destAccountKey, setDestAccountValue)
		return nil, nil
	})

	return merry.Wrap(err)
}

// FetchAccounts - получить список аккаунтов
func (cluster *FDBCluster) FetchAccounts() ([]model.Account, error) {
	var accounts []model.Account
	var accountKeyValueArray []fdb.KeyValue

	// StreamingModeWantAll - режим "прочитать всё и как можно быстрее"
	data, err := cluster.pool.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		r := tx.GetRange(cluster.model.accounts, fdb.RangeOptions{Limit: 0, Mode: fdb.StreamingModeWantAll, Reverse: false}).Iterator()
		for r.Advance() {
			accountKeyValue, err := r.Get()
			if err != nil {
				return nil, merry.Wrap(err)
			}
			// для скорости обработки возвращаем необработанный массив пар ключ-значение
			accountKeyValueArray = append(accountKeyValueArray, accountKeyValue)
		}
		llog.Traceln("count of elements inside loop of transaction", len(accountKeyValueArray))
		return accountKeyValueArray, nil
	})
	if err != nil {
		return nil, merry.Prepend(err, "failed to fetch accounts")
	}

	accountKeyValues, ok := data.([]fdb.KeyValue)
	if !ok {
		return nil, merry.Errorf("this type data of fdb.KeyValue is not supported")
	}

	llog.Traceln("count of elements outside loop of transaction", len(accountKeyValueArray))
	for _, accountKeyValue := range accountKeyValues {
		keyAccountTuple, err := cluster.model.accounts.Unpack(accountKeyValue.Key)
		if err != nil {
			return nil, merry.Prepend(err, "failed to unpack by key in FetchAccounts FDB")
		}

		var fetchAccount model.Account
		var fetchAccountValue accountValue
		if len(accountKeyValue.Value) == 0 {
			return nil, ErrNoRows
		}
		fetchAccountValue, err = deserializeAccountValue(accountKeyValue.Value)
		if err != nil {
			return nil, err
		}

		fetchAccount.Balance = fetchAccountValue.Balance
		Bic, ok := keyAccountTuple[0].(string)
		if !ok {
			return nil, merry.Errorf("account bic is not string, value: %v \n", fetchAccount.Bic)
		}
		fetchAccount.Bic = Bic
		Ban, ok := keyAccountTuple[1].(string)
		if !ok {
			return nil, merry.Errorf("account bic is not string, value: %v \n", fetchAccount.Ban)
		}
		fetchAccount.Ban = Ban
		accounts = append(accounts, fetchAccount)
	}

	return accounts, nil
}

// FetchBalance - получить баланс счета по атрибутам ключа счета.
func (cluster *FDBCluster) FetchBalance(bic string, ban string) (*inf.Dec, *inf.Dec, error) {
	var balances, pendingAmount *inf.Dec
	data, err := cluster.pool.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		fetchAccount := model.Account{
			Bic:             bic,
			Ban:             ban,
			Balance:         balances,
			PendingAmount:   pendingAmount,
			PendingTransfer: [16]byte{},
			Found:           false,
		}
		var fetchAccountValue accountValue
		var balance *inf.Dec
		keyFetchAccount := cluster.getAccountKey(fetchAccount)
		fetchAccountValue, err := getAccountValue(tx, keyFetchAccount)
		if err != nil {
			return nil, merry.Prepend(err, "failed to get balance value in FetchBalance FDB")
		}
		balance = fetchAccountValue.Balance
		return balance, nil
	})
	if err != nil {
		return nil, nil, merry.Wrap(err)
	}
	balances, ok := data.(*inf.Dec)
	if !ok {
		return nil, nil, merry.Errorf("balance data type is not supported, value: %v", balances)
	}

	return balances, pendingAmount, nil
}

func serializeValue(prepare interface{}) (pack []byte, errs error) {
	pack, err := json.Marshal(prepare)
	if err != nil {
		return nil, merry.Wrap(err)
	}
	return pack, nil
}

func serializeCheckSum(prepare inf.Dec) (pack []byte, errs error) {
	/*inf.Dec взятое отдельно, через json нормально не сериализуется, поэтому используем
	штатное преобразование из библиотеки inf.v0*/
	pack, err := prepare.GobEncode()
	if err != nil {
		return nil, merry.Wrap(err)
	}
	return pack, nil
}

func deserializeCheckSum(value []byte) (postmap inf.Dec, errs error) {
	var result inf.Dec
	if len(value) == 0 {
		return result, ErrNoRows
	}
	/*inf.Dec взятое отдельно, через json нормально не десериализуется, поэтому используем
	штатное преобразование из библиотеки inf.v0*/
	err := result.GobDecode(value)
	if err != nil {
		return result, merry.Wrap(err)
	}
	return result, nil
}

func deserializeAccountValue(value []byte) (postmap accountValue, errs error) {
	err := json.Unmarshal(value, &postmap)
	if err != nil {
		return postmap, merry.Wrap(err)
	}
	return postmap, nil
}

// getAccountValue - получить атрибуты счета (value) по параметрам счета (key). Вариант для транзакции с записью.
func getAccountValue(tx fdb.ReadTransaction, key fdb.Key) (valueResult accountValue, err error) {
	valueSrc, err := tx.Get(key).Get()
	if err != nil {
		return valueResult, merry.Wrap(err)
	}
	if len(valueSrc) == 0 {
		return valueResult, ErrNoRows
	}
	valueResult, err = deserializeAccountValue(valueSrc)
	if err != nil {
		return valueResult, err
	}
	return valueResult, nil
}

/*метод не возвращает пару ключ-ошибка, т.к. метод Pack не возвращает ошибку -
мы формируем идентификатор ключа в любом случае,
проверка на наличие ключа есть в методах получения, вернется nil, если по ключу ничего не найдено*/
// getAccountKey - cформировать ключ по параметрам счета.
func (cluster *FDBCluster) getAccountKey(space model.Account) (keyResult fdb.Key) {
	keyResult = cluster.model.accounts.Pack(tuple.Tuple{space.Bic, space.Ban})
	return keyResult
}

/*метод не возвращает пару ключ-ошибка, т.к. метод Pack не возвращает ошибку -
мы формируем идентификатор ключа в любом случае,
проверка на наличие ключа есть в методах получения, вернется nil,
если по ключу ничего не найдено, нужно это обрабатывать*/
// getTransferKey - сформировать ключ по параметрам перевода.
func (cluster *FDBCluster) getTransferKey(space *model.Transfer) (keyResult fdb.Key) {
	/*tuple не поддерживает тип uuid.UUID,
	доступные типы https://github.com/apple/foundationdb/blob/master/bindings/go/src/fdb/tuple/tuple.go
	согласно доке на tuple.UUID:
	 this simple wrapper allows for other libraries to write the output of their UUID type as a 16-byte array into
	an instance of this type.*/
	spaceID := tuple.UUID(space.Id)
	keyResult = cluster.model.transfers.Pack(
		tuple.Tuple{
			spaceID,
			space.Acs[0].Bic,
			space.Acs[0].Ban,
			space.Acs[1].Bic,
			space.Acs[1].Ban,
		})
	return keyResult
}

// setTransfer - сделать перевод.
func (cluster *FDBCluster) setTransfer(tx fdb.Transaction, transfer *model.Transfer) error {
	var preparemap transferValue
	preparemap.Amount = transfer.Amount
	transferValue, err := serializeValue(preparemap)
	if err != nil {
		return merry.Prepend(err, "failed to serialize value transfer")
	}
	transferKey := cluster.getTransferKey(transfer)
	tx.Set(transferKey, transferValue)
	return err
}

func (cluster *FDBCluster) GetStatistics() error {
	errChan := make(chan error)

	llog.Debugln("starting of statistic goroutine...")
	go cluster.getStatistics(errChan)

	errorCheck := <-errChan

	if errorCheck != nil {
		return merry.Prepend(errorCheck, "failed to get statistic")
	}

	return nil
}

func (cluster *FDBCluster) getStatistics(errChan chan error) {
	var once sync.Once
	var resultMap map[string]interface{}
	var jsonResult []byte

	const dateFormat = "02-01-2006_15:04:05"

	statFileName := fmt.Sprintf(statJsonFileTemplate, time.Now().Format(dateFormat))
	llog.Debugln("Opening statistic file...")
	statFile, err := os.OpenFile(statFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		errChan <- merry.Prepend(err, "failed to open statistic file")
	}

	defer statFile.Close()

	llog.Debugln("Opening statistic file: success")

	for {
		data, err := cluster.pool.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
			status, err := tx.Get(fdb.Key("\xFF\xFF/status/json")).Get()
			if err != nil {
				return nil, err
			}
			return status, nil
		})
		if err != nil {
			errChan <- merry.Prepend(err, "failed to get status json from db")
		}

		result, ok := data.([]byte)
		if !ok {
			errChan <- merry.Errorf("status data type is not supported, value: %v", result)
		}

		if err = json.Unmarshal(result, &resultMap); err != nil {
			errChan <- merry.Prepend(err, "failed to unmarchal status json")
		}

		separateString := fmt.Sprintf("\n %v \n", time.Now().Format(dateFormat))
		if _, err = statFile.Write([]byte(separateString)); err != nil {
			errChan <- merry.Prepend(err, "failed to write separate string to statistic file")
		}

		if jsonResult, err = json.MarshalIndent(resultMap, "", "    "); err != nil {
			errChan <- merry.Prepend(err, "failed to marshal data")
		}

		if _, err = statFile.Write(jsonResult); err != nil {
			errChan <- merry.Prepend(err, "failed to write data to statistic file")
		}

		// если ошибки нет, то отправляем nil, чтобы продолжить работу
		once.Do(func() {
			errChan <- nil
		})

		time.Sleep(30 * time.Second)
	}
}
