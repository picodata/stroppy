/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package cluster

import (
	"errors"
	"time"

	"github.com/ansel1/merry"
	"github.com/google/uuid"
	llog "github.com/sirupsen/logrus"
	"github.com/tarantool/go-tarantool"
	"gitlab.com/picodata/stroppy/internal/model"
	"gopkg.in/inf.v0"
)

// TarantoolCluster - объявление соединения к FDB и ссылки на модель данных.
type TarantoolCluster struct {
	conn *tarantool.Connection
}

type tarantoolModel struct {
}

type settingsParams struct {
	Key   string
	Value int
}

type totalBalance struct {
	Name  string
	Total int64
}

type finalBalance struct {
	Balance int64
}

type transactionResult struct {
	Result string
}

func (cluster *TarantoolCluster) InsertTransfer(_ *model.Transfer) error {
	return errors.New("implement me")
}

func (cluster *TarantoolCluster) DeleteTransfer(_ model.TransferId, _ uuid.UUID) error {
	return errors.New("implement me")
}

func (cluster *TarantoolCluster) SetTransferClient(clientId uuid.UUID, transferId model.TransferId) error {
	panic("implement me")
}

func (cluster *TarantoolCluster) FetchTransferClient(transferId model.TransferId) (*uuid.UUID, error) {
	panic("implement me")
}

func (cluster *TarantoolCluster) ClearTransferClient(transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cluster *TarantoolCluster) SetTransferState(state string, transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cluster *TarantoolCluster) FetchTransfer(transferId model.TransferId) (*model.Transfer, error) {
	panic("implement me")
}

func (cluster *TarantoolCluster) FetchDeadTransfers() ([]model.TransferId, error) {
	panic("implement me")
}

func (cluster *TarantoolCluster) UpdateBalance(balance *inf.Dec, bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

func (cluster *TarantoolCluster) LockAccount(transferId model.TransferId, pendingAmount *inf.Dec, bic string, ban string) (*model.Account, error) {
	panic("implement me")
}

func (cluster *TarantoolCluster) UnlockAccount(bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

// NewFoundationCluster - Создать подключение к Tarantool и создать новые коллекции, если ещё не созданы.
func NewTarantoolCluster(dbURL string, poolSize uint64, sharded bool) (*TarantoolCluster, error) {
	opts := tarantool.Opts{User: "stroppy", Pass: "stroppy", Concurrency: uint32(poolSize)}

	llog.Debugln("connecting to tarantool...")
	conn, err := tarantool.Connect(dbURL, opts)
	if err != nil {
		return nil, merry.Prepend(err, "failed to connect tarantool database")
	}

	llog.Debugf("Initialed %v mutexes for requests \n", opts.Concurrency)

	if _, err = conn.Call("box.schema.space.create", []interface{}{"accounts", map[string]bool{"if_not_exists": true}}); err != nil {
		return nil, merry.Prepend(err, "failed to create space account")
	}

	if _, err = conn.Call("box.schema.space.create", []interface{}{"transfers", map[string]bool{"if_not_exists": true}}); err != nil {
		return nil, merry.Prepend(err, "failed to create space transfers")
	}

	if _, err = conn.Call("box.schema.space.create", []interface{}{"settings", map[string]bool{"if_not_exists": true}}); err != nil {
		return nil, merry.Prepend(err, "failed to create space settings")
	}

	if _, err = conn.Call("box.schema.space.create", []interface{}{"checksum", map[string]bool{"if_not_exists": true}}); err != nil {
		return nil, merry.Prepend(err, "failed to create space checksum")
	}

	if _, err = conn.Call("box.space.accounts:format", [][]map[string]string{
		{
			{"name": "bic", "type": "string"},
			{"name": "ban", "type": "string"},
			{"name": "balance", "type": "number"},
		}}); err != nil {
		return nil, merry.Prepend(err, "failed to format accounts space")
	}

	if _, err = conn.Call("box.space.accounts:create_index", []interface{}{
		"primary",
		map[string]interface{}{
			"parts":         []string{"bic", "ban"},
			"if_not_exists": true}}); err != nil {
		return nil, merry.Prepend(err, "failed to create primary index for accounts space")
	}

	if _, err = conn.Call("box.space.transfers:format", [][]map[string]string{
		{
			{"name": "transfer_id", "type": "uuid"},
			{"name": "src_bic", "type": "string"},
			{"name": "src_ban", "type": "string"},
			{"name": "dest_bic", "type": "string"},
			{"name": "dest_ban", "type": "string"},
			{"name": "balance", "type": "number"},
		}}); err != nil {
		return nil, merry.Prepend(err, "failed to format transfers space")
	}

	if _, err = conn.Call("box.space.transfers:create_index", []interface{}{
		"primary",
		map[string]interface{}{
			"parts":         []string{"transfer_id"},
			"if_not_exists": true}}); err != nil {
		return nil, merry.Prepend(err, "failed to create primary index for accounts space")
	}

	if _, err = conn.Call("box.space.settings:format", [][]map[string]string{
		{
			{"name": "key", "type": "string"},
			{"name": "value", "type": "number"},
		}}); err != nil {
		return nil, merry.Prepend(err, "failed to format settings space")
	}

	if _, err = conn.Call("box.space.settings:create_index", []interface{}{
		"primary",
		map[string]interface{}{
			"parts":         []string{"key"},
			"if_not_exists": true}}); err != nil {
		return nil, merry.Prepend(err, "failed to create primary index for settings space")
	}

	if _, err = conn.Call("box.space.checksum:format", [][]map[string]string{
		{
			{"name": "name", "type": "string"},
			{"name": "amount", "type": "number"},
		}}); err != nil {
		return nil, merry.Prepend(err, "failed to format checksum space")
	}

	if _, err = conn.Call("box.space.checksum:create_index", []interface{}{
		"primary",
		map[string]interface{}{
			"parts":         []string{"name"},
			"if_not_exists": true}}); err != nil {
		return nil, merry.Prepend(err, "failed to create primary index for checksum space")
	}

	return &TarantoolCluster{
		conn: conn,
	}, nil
}

func (cluster *TarantoolCluster) addSharding() error {
	return nil
}

// BootstrapDB - заполнить параметры настройки  и инициализировать ключ для хранения итогового баланса.
func (cluster *TarantoolCluster) BootstrapDB(count int, seed int) error {
	var err error
	llog.Infof("Populating settings...")

	if _, err = cluster.conn.Call("box.space.accounts:truncate", []interface{}{}); err != nil {
		return merry.Prepend(err, "failed to truncate space account")
	}

	if _, err = cluster.conn.Call("box.space.transfers:truncate", []interface{}{}); err != nil {
		return merry.Prepend(err, "failed to truncate space transfers")
	}

	if _, err = cluster.conn.Call("box.space.settings:truncate", []interface{}{}); err != nil {
		return merry.Prepend(err, "failed to truncate space settings")
	}

	if _, err = cluster.conn.Call("box.space.checksum:truncate", []interface{}{}); err != nil {
		return merry.Prepend(err, "failed to truncate space checksum")
	}

	resp, err := cluster.conn.Insert("settings", []interface{}{"count", count})
	if err != nil {
		return merry.Errorf("failed to insert count in settings. Err: %v, Resp: %v %v", err, resp.Code, resp.Data)
	}

	resp, err = cluster.conn.Insert("settings", []interface{}{"seed", seed})
	if err != nil {
		return merry.Errorf("failed to insert seed in settings. Err: %v, Resp: %v %v", err, resp.Code, resp.Data)
	}

	if _, err = cluster.conn.Eval(SumAccountsBalancesFunction, []interface{}{}); err != nil {
		return merry.Prepend(err, "failed to create function for accounts balances sum")
	}

	if _, err = cluster.conn.Eval(MakeAtomicTransferFunction, []interface{}{}); err != nil {
		return merry.Prepend(err, "failed to create function for atomic transfer")
	}

	return nil
}

// GetClusterType - получить тип DBCluster.
func (cluster *TarantoolCluster) GetClusterType() DBClusterType {
	return TarantoolClusterType
}

// FetchSettings - получить значения параметров настройки.
func (cluster *TarantoolCluster) FetchSettings() (Settings, error) {
	//var count, seed int
	var results []settingsParams

	err := cluster.conn.SelectTyped("settings", "primary", 0, 2, tarantool.IterAll, []interface{}{}, &results)
	if err != nil {
		return Settings{}, merry.Errorf("failed to select settings from account. Err: %v", err)
	}

	return Settings{
		Count: results[0].Value,
		Seed:  results[1].Value,
	}, nil
}

// InsertAccount - сохранить новый счет.
func (cluster *TarantoolCluster) InsertAccount(acc model.Account) (err error) {

	resp, err := cluster.conn.Insert("accounts", []interface{}{acc.Bic, acc.Ban, acc.Balance.UnscaledBig().Int64()})
	if err != nil {
		if tntErr, ok := err.(tarantool.Error); ok || tntErr.Code == tarantool.ErrTupleFound {
			return ErrDuplicateKey
		}
		return merry.Errorf("failed to insert record in account. Err: %v, Resp: %v %v", err, resp.Code, resp.Data)
	}

	return nil
}

// FetchTotal - получить значение итогового баланса из Settings.
func (cluster *TarantoolCluster) FetchTotal() (*inf.Dec, error) {
	var results []totalBalance
	var balance *inf.Dec

	err := cluster.conn.SelectTyped("checksum", "primary", 0, 1, tarantool.IterAll, []interface{}{}, &results)
	if err != nil {
		return nil, merry.Errorf("failed to select total from checksum. Err: %v", err)
	}

	if len(results) != 0 {
		balance = inf.NewDec(results[0].Total, 0)
	} else {
		return nil, ErrNoRows
	}

	return balance, nil
}

// PersistTotal - сохранить значение итогового баланса в settings.
func (cluster *TarantoolCluster) PersistTotal(total inf.Dec) error {

	resp, err := cluster.conn.Insert("checksum", []interface{}{"total", total.UnscaledBig().Int64()})
	if err != nil {
		return merry.Errorf("failed to insert total balance in checksum. Err: %v, Resp: %v %v", err, resp.Code, resp.Data)
	}

	return nil
}

// CheckBalance - рассчитать итоговый баланc.
func (cluster *TarantoolCluster) CheckBalance() (*inf.Dec, error) {
	var result []finalBalance

	err := cluster.conn.CallTyped("sum_accounts_balances", []interface{}{}, &result)
	if err != nil {
		return nil, merry.Prepend(err, "failed to calculate total balance from accounts")
	}

	llog.Infoln(result[0].Balance)

	return inf.NewDec(result[0].Balance, 0), nil
}

// MakeAtomicTransfer - выполнить операцию перевода и изменить балансы source и dest cчетов.
func (cluster *TarantoolCluster) MakeAtomicTransfer(transfer *model.Transfer) error {
	var result []transactionResult

	if err := cluster.conn.CallTyped("makeAtomicTransfer", []interface{}{transfer.Id.String(), transfer.Acs[0].Bic, transfer.Acs[0].Ban, transfer.Acs[1].Bic, transfer.Acs[1].Ban,
		transfer.Amount.UnscaledBig().Int64()}, &result); err != nil {
		return merry.Prepend(err, "failed to execute transaction")
	}

	if result[0].Result == "ErrInsufficientFunds" {
		return ErrInsufficientFunds
	}

	if result[0].Result == "ErrNotFound" {
		return ErrNoRows
	}

	return nil
}

// FetchAccounts - получить список аккаунтов
func (cluster *TarantoolCluster) FetchAccounts() ([]model.Account, error) {
	return nil, nil
}

// FetchBalance - получить баланс счета по атрибутам ключа счета.
func (cluster *TarantoolCluster) FetchBalance(bic string, ban string) (*inf.Dec, *inf.Dec, error) {
	return nil, nil, nil
}

func (cluster *TarantoolCluster) StartStatisticsCollect(statInterval time.Duration) error {
	llog.Infoln("collect statistic from tarantool db is not implemented")
	return nil
}
