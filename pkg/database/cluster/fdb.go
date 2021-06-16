package cluster

import (
	"encoding/json"
	"errors"
	"strconv"

	"gitlab.com/picodata/stroppy/internal/model"

	"github.com/ansel1/merry"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	llog "github.com/sirupsen/logrus"
	"gopkg.in/inf.v0"
)

const versionAPI = 620

// FDBCluster - объявление соединения к FDB и ссылки на модель данных.
type FDBCluster struct {
	pool  fdb.Database
	model modelFDB
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

// NewFDBCluster - Создать подключение к FDB и создать новые DirectorySubspace, если ещё не созданы.
func NewFDBCluster(dbURL string) (*FDBCluster, error) {
	llog.Infof("Establishing connection to FDB on %v", dbURL)
	poolConfig := dbURL
	err := fdb.APIVersion(versionAPI)
	if err != nil {
		return nil, merry.Prepend(err, "failed to check version FDB API")
	}
	FDBPool, err := fdb.OpenDatabase(poolConfig)
	if err != nil {
		return nil, merry.Prepend(err, "failed to open connect to FDB")
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
func (cluster *FDBCluster) FetchSettings() (ClusterSettings, error) {
	var clusterSettings ClusterSettings
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
		return ClusterSettings{
			Count: fetchCount,
			Seed:  fetchSeed,
		}, nil
	})
	if err != nil {
		// не удается вернуть nil, возникает ошибка
		return clusterSettings, merry.Prepend(err, "failed to fetch from Settings")
	}
	clusterSettings, ok := data.(ClusterSettings)
	if !ok {
		return clusterSettings, merry.Errorf("this data type ClusterSettings is not supported")
	}
	return clusterSettings, nil
}

// InsertAccount - сохранить новый счет.
func (cluster *FDBCluster) InsertAccount(acc model.Account) error {
	data, err := cluster.pool.Transact(func(tx fdb.Transaction) (interface{}, error) {
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
		return valueAccount.Balance, err
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateKey) {
			return ErrDuplicateKey
		}
		return merry.Prepend(err, "failed to insert account")
	}

	initialBalance := inf.NewDec(0, 10)

	addedBalance, ok := data.(*inf.Dec)
	if !ok {
		return merry.Prepend(err, "this data type account balance is not supported")
	}

	currentBalance, err := cluster.FetchTotal()

	if err != nil {
		if !errors.Is(err, ErrNoRows) {
			return merry.Prepend(err, "failed to get current total balance")
		}
		//если мы выполняемся первый раз и пришло пустое значение, то инициализируем нулем
		currentBalance = initialBalance
	}

	// добавляем баланс аккаунта к общему балансу
	currentBalance.Add(currentBalance, addedBalance)

	err = cluster.PersistTotal(*currentBalance)
	if err != nil {
		return merry.Prepend(err, "failed to keep total balance")
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
	var fetchResult []model.Account
	// присваиваем ноль, т.к. инициализируется как nil, иначе не сработает расчет итогового баланса
	amount := inf.NewDec(0, 10)

	fetchResult, err := cluster.FetchAccounts()
	if err != nil {
		return amount, merry.Prepend(err, "failed to get Accounts array")
	}
	for i := range fetchResult {
		fetchAccount := fetchResult[i]
		amount = amount.Add(amount, fetchAccount.Balance)
	}
	return amount, nil
}

// MakeAtomicTransfer - выполнить операцию перевода и изменить балансы source и dest cчетов.
func (cluster *FDBCluster) MakeAtomicTransfer(transfer *model.Transfer) error {
	tx, err := cluster.pool.CreateTransaction()
	if err != nil {
		tx.Cancel()
		return merry.Prepend(err, "failed to create tx FDB")
	}
	err = cluster.setTransfer(tx, transfer)
	if err != nil {
		tx.Cancel()
		return ErrTxRollback
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
			tx.Cancel()
			return ErrNoRows
		}
		tx.Cancel()
		return merry.Prepend(err, "failed to get source account")
	}
	sourceAccountValue.Balance.Sub(sourceAccountValue.Balance, transfer.Amount)
	if sourceAccountValue.Balance.UnscaledBig().Int64() < 0 {
		tx.Cancel()
		return ErrInsufficientFunds
	}
	setSourceAccountValue, err := serializeValue(sourceAccountValue)
	if err != nil {
		tx.Cancel()
		return merry.Prepend(err, "failed to serialize source account")
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
			tx.Cancel()
			return ErrNoRows
		}
		tx.Cancel()
		return merry.Prepend(err, "failed to get destination account")
	}
	destAccountValue.Balance.Add(destAccountValue.Balance, transfer.Amount)
	setDestAccountValue, err := serializeValue(destAccountValue)
	if err != nil {
		tx.Cancel()
		return merry.Prepend(err, "failed to serialize destination account")
	}
	tx.Set(destAccountKey, setDestAccountValue)
	err = tx.Commit().Get()
	if err != nil {
		tx.Cancel()
		return ErrTxRollback
	}
	return nil
}

// FetchAccounts - получить список аккаунтов
func (cluster *FDBCluster) FetchAccounts() ([]model.Account, error) {
	var accounts []model.Account
	var accountKeyValuesArray []fdb.KeyValue

	beginKey, endKey := cluster.model.accounts.FDBRangeKeys()

	settings, err := cluster.FetchSettings()
	if err != nil {
		return nil, merry.Prepend(err, "failed to fetch settings for fetch accounts")
	}

	var data interface{}
	if settings.Count > 10000 && settings.Count%10000 == 0 {

		for i := 0; i < settings.Count; i = i + 10000 {
			data, err = cluster.pool.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
				keyValueArray, err := tx.GetRange(fdb.KeyRange{
					Begin: beginKey,
					End:   endKey,
				},
					fdb.RangeOptions{Limit: 10000, Mode: fdb.StreamingModeWantAll, Reverse: false}).GetSliceWithError()
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

			/*
				добавляем полученную порцию в массив, затем получаем следующий за последним элементом массива
				ключ и задаем его началом следующего диапазона
			*/
			accountKeyValuesArray = append(accountKeyValuesArray, accountsKeyValues...)
			selectorkey := fdb.FirstGreaterOrEqual(accountKeyValuesArray[len(accountKeyValuesArray)-1].Key)
			beginKey = selectorkey.Key

		}
	}
	for _, accountKeyValue := range accountKeyValuesArray {
		accountKeyTuple, err := cluster.model.accounts.Unpack(accountKeyValue.Key)
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
		Bic, ok := accountKeyTuple[0].(string)
		if !ok {
			return nil, merry.Errorf("account bic is not string, value: %v \n", fetchAccount.Bic)
		}
		fetchAccount.Bic = Bic
		Ban, ok := accountKeyTuple[1].(string)
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
