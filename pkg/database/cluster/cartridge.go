/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package cluster

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/tarantool/go-tarantool"

	"github.com/ansel1/merry"
	"github.com/google/uuid"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/internal/model"
	"gopkg.in/inf.v0"
)

// CartridgeCluster - объявление соединения к FDB и ссылки на модель данных.
type CartridgeCluster struct {
	client      *http.Client
	binary_conn *tarantool.Connection
	url         string
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

type account struct {
	Bic             string `json:"bic"`
	Ban             string `json:"ban"`
	Balance         int64  `json:"balance"`
	PendingAmount   int64  `json:"pending_amount"`
	PendingTransfer uuid.UUID `json:"pending_transfer"`
	Found           bool `json:"found"`
}

func (cluster *CartridgeCluster) InsertTransfer(_ *model.Transfer) error {
	return errors.New("implement me")
}

func (cluster *CartridgeCluster) DeleteTransfer(_ model.TransferId, _ uuid.UUID) error {
	return errors.New("implement me")
}

func (cluster *CartridgeCluster) SetTransferClient(clientId uuid.UUID, transferId model.TransferId) error {
	panic("implement me")
}

func (cluster *CartridgeCluster) FetchTransferClient(transferId model.TransferId) (*uuid.UUID, error) {
	panic("implement me")
}

func (cluster *CartridgeCluster) ClearTransferClient(transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cluster *CartridgeCluster) SetTransferState(state string, transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cluster *CartridgeCluster) FetchTransfer(transferId model.TransferId) (*model.Transfer, error) {
	panic("implement me")
}

func (cluster *CartridgeCluster) FetchDeadTransfers() ([]model.TransferId, error) {
	panic("implement me")
}

func (cluster *CartridgeCluster) UpdateBalance(balance *inf.Dec, bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

func (cluster *CartridgeCluster) LockAccount(transferId model.TransferId, pendingAmount *inf.Dec, bic string, ban string) (*model.Account, error) {
	panic("implement me")
}

func (cluster *CartridgeCluster) UnlockAccount(bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

// NewFoundationCluster - Создать подключение к cartridge и создать новые коллекции, если ещё не созданы.
func NewCartridgeCluster(dbURL string, poolSize uint64, sharded bool) (*CartridgeCluster, error) {

	llog.Debugln("connecting to cartridge...")

	transport := &http.Transport{
		MaxConnsPerHost: int(poolSize),
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   socketTimeout,
	}

	return &CartridgeCluster{
		client: client,
		url:    dbURL,
	}, nil
}

func (cluster *CartridgeCluster) addSharding() error {
	return nil
}

// BootstrapDB - заполнить параметры настройки  и инициализировать ключ для хранения итогового баланса.
func (cluster *CartridgeCluster) BootstrapDB(count int, seed int) error {
	llog.Infof("Populating settings...")

	body_template := []byte(`{"count":` + fmt.Sprintf("%v", count) + `,"seed": ` + fmt.Sprintf("%v", seed) + `}`)
	request_template := fmt.Sprintf("%s/bootstrap_db", cluster.url)

	request, err := http.NewRequest(
		"POST", request_template, bytes.NewBuffer(body_template),
	)

	if err != nil {
		return merry.Prepend(err, "failed to create request of bootstrap DB in cartridge app")
	}

	request.Header.Set("Content-Type", "application/json")

	resp, err := cluster.client.Do(request)

	if err != nil {
		return merry.Prepend(err, "failed to make request of bootstrap DB in cartridge app")
	}

	if resp.StatusCode != 200 {
		return merry.Prepend(err, "failed to bootstrap DB in cartridge app")
	}

	defer resp.Body.Close()

	return nil
}

// GetClusterType - получить тип DBCluster.
func (cluster *CartridgeCluster) GetClusterType() DBClusterType {
	return CartridgeClusterType
}

// FetchSettings - получить значения параметров настройки.
func (cluster *CartridgeCluster) FetchSettings() (Settings, error) {

	request_template := fmt.Sprintf("%s/fetch_settings", cluster.url)
	request, err := http.NewRequest(
		"GET", request_template, nil,
	)

	if err != nil {
		return Settings{}, merry.Prepend(err, "failed to create request to fetch settings in cartridge app")
	}

	resp, err := cluster.client.Do(request)

	if err != nil {
		return Settings{}, merry.Prepend(err, "failed to make request to fetch settings in cartridge app")
	}

	if resp.StatusCode != 200 {
		return Settings{}, merry.Prepend(err, "failed to fetch settings in cartridge app")
	}

	defer resp.Body.Close()

	var settings map[string]map[string]int

	if err = json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return Settings{}, merry.Prepend(err, "failed to decode settings from cartridge app response")
	}

	return Settings{
		Count: settings["info"]["count"],
		Seed:  settings["info"]["seed"],
	}, nil
}

// InsertAccount - сохранить новый счет.
func (cluster *CartridgeCluster) InsertAccount(acc model.Account) (err error) {

	account := account{
		Bic:             acc.Bic,
		Ban:             acc.Ban,
		Balance:         acc.Balance.UnscaledBig().Int64(),
		PendingAmount:   acc.PendingAmount.UnscaledBig().Int64(),
		PendingTransfer: acc.PendingTransfer,
		Found:           false,
	}

	account_json, err := json.Marshal(account)
	if err != nil {
		return merry.Prepend(err, "failed to marshal account to json to insert account in cartridge app")
	}

	llog.Traceln(string(account_json))

	request_template := fmt.Sprintf("%s/insert_account", cluster.url)

	request, err := http.NewRequest(
		"POST", request_template, bytes.NewBuffer(account_json),
	)

	if err != nil {
		return merry.Prepend(err, "failed to create request to insert account in cartridge app")
	}

	request.Header.Set("Content-Type", "application/json")

	resp, err := cluster.client.Do(request)

	if err != nil {
		return merry.Prepend(err, "failed to make request to insert account in cartridge app")
	}

	if resp.StatusCode != 201 && resp.StatusCode != 409 {
		error_body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return merry.Prepend(err, "failed to insert account in cartridge app, but error body cannot be read")
		}
		return merry.Errorf("failed to insert account in cartridge app: %v %v", resp.StatusCode, string(error_body))
	}

	var response map[string]string

	if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return merry.Prepend(err, "failed to decode result to insert account from cartridge app response")
	}

	if resp.StatusCode == 409 {
		if response["error"] == "Account already exist" {
			return ErrDuplicateKey
		}
	}

	defer resp.Body.Close()

	return nil
}

// FetchTotal - получить значение итогового баланса из Settings.
func (cluster *CartridgeCluster) FetchTotal() (*inf.Dec, error) {
	request_template := fmt.Sprintf("%s/fetch_total", cluster.url)
	request, err := http.NewRequest(
		"GET", request_template, nil,
	)

	if err != nil {
		return nil, merry.Prepend(err, "failed to create request to fetch total balance in cartridge app")
	}

	resp, err := cluster.client.Do(request)

	if err != nil {
		return nil, merry.Prepend(err, "failed to make request to fetch total balance in cartridge app")
	}

	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			return nil, ErrNoRows
		}
		return nil, merry.Prepend(err, "failed to fetch total balance in cartridge app")
	}

	defer resp.Body.Close()

	var balance_from_resp map[string]string

	if err = json.NewDecoder(resp.Body).Decode(&balance_from_resp); err != nil {
		return nil, merry.Prepend(err, "failed to fetch total balance from cartridge app response")
	}

	llog.Debugf("result of fetch balance %v \n", balance_from_resp["info"])

	balance, err := strconv.ParseInt(balance_from_resp["info"], 0, 64)
	if err != nil {
		return nil, merry.Prepend(err, "failed to convert balance string from cartridge app response")
	}

	return inf.NewDec(balance, 0), nil
}

// PersistTotal - сохранить значение итогового баланса в settings.
func (cluster *CartridgeCluster) PersistTotal(total inf.Dec) error {
	body_template := []byte(`{"total":` + fmt.Sprintf("%v", total.UnscaledBig().Int64()) + `}`)
	request_template := fmt.Sprintf("%s/persist_total", cluster.url)

	request, err := http.NewRequest(
		"POST", request_template, bytes.NewBuffer(body_template),
	)

	if err != nil {
		return merry.Prepend(err, "failed to create request to persist total balance in cartridge app")
	}

	request.Header.Set("Content-Type", "application/json")

	resp, err := cluster.client.Do(request)

	if err != nil {
		return merry.Prepend(err, "failed to make request to persist total balance in cartridge app")
	}

	var response map[string]interface{}

	if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return merry.Prepend(err, "failed to persist total balance in cartridge app")
	}

	defer resp.Body.Close()

	return nil
}

// CheckBalance - рассчитать итоговый баланc.
func (cluster *CartridgeCluster) CheckBalance() (*inf.Dec, error) {
	request_template := fmt.Sprintf("%s/check_balance", cluster.url)
	request, err := http.NewRequest(
		"GET", request_template, nil,
	)

	if err != nil {
		return nil, merry.Prepend(err, "failed to create request to calculate balance in cartridge app")
	}

	resp, err := cluster.client.Do(request)

	if err != nil {
		return nil, merry.Prepend(err, "failed to make request to calculate balance in cartridge app")
	}

	if resp.StatusCode != 200 {
		return nil, merry.Prepend(err, "failed to calculate balance in cartridge app")
	}

	defer resp.Body.Close()

	var balance_from_resp map[string]string

	if err = json.NewDecoder(resp.Body).Decode(&balance_from_resp); err != nil {
		return nil, merry.Prepend(err, "failed to calculate balance from cartridge app response")
	}

	llog.Debugf("result of check balance %v \n", balance_from_resp["info"])

	balance, err := strconv.ParseInt(balance_from_resp["info"], 0, 64)
	if err != nil {
		return nil, merry.Prepend(err, "failed to convert balance string from cartridge app response")
	}

	return inf.NewDec(balance, 0), nil
}

// MakeAtomicTransfer - выполнить операцию перевода и изменить балансы source и dest cчетов.
func (cluster *CartridgeCluster) MakeAtomicTransfer(transfer *model.Transfer) error {

	transfer_json, err := json.Marshal(transfer)
	if err != nil {
		merry.Prepend(err, "failed to marshal transfer to json to make custom transfer in cartridge app")
	}

	llog.Debugln(string(transfer_json))

	request_template := fmt.Sprintf("%s/make_custom_transfer", cluster.url)

	request, err := http.NewRequest(
		"POST", request_template, bytes.NewBuffer(transfer_json),
	)

	if err != nil {
		return merry.Prepend(err, "failed to create request to make custom transfer in cartridge app")
	}

	request.Header.Set("Content-Type", "application/json")

	resp, err := cluster.client.Do(request)

	if err != nil {
		return merry.Prepend(err, "failed to make request to make custom transfer in cartridge app")
	}

	var response map[string]interface{}

	if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return merry.Prepend(err, "failed to make custom transfer balance in cartridge app")
	}

	defer resp.Body.Close()

	return nil
}

// FetchAccounts - получить список аккаунтов
func (cluster *CartridgeCluster) FetchAccounts() ([]model.Account, error) {
	return nil, nil
}

// FetchBalance - получить баланс счета по атрибутам ключа счета.
func (cluster *CartridgeCluster) FetchBalance(bic string, ban string) (*inf.Dec, *inf.Dec, error) {
	return nil, nil, nil
}

func (cluster *CartridgeCluster) StartStatisticsCollect(statInterval time.Duration) error {
	llog.Infoln("collect statistic from cartridge db is not implemented")
	return nil
}
