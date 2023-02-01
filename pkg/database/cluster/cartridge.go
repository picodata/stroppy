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
//
//nolint:golint,structcheck
type CartridgeCluster struct {
	client *http.Client
	//nolint:golint,unused
	binary_conn *tarantool.Connection
	url         string
}

type account struct {
	Bic             string    `json:"bic"`
	Ban             string    `json:"ban"`
	Balance         int64     `json:"balance"`
	PendingAmount   int64     `json:"pending_amount"`
	PendingTransfer uuid.UUID `json:"pending_transfer"`
	Found           bool      `json:"found"`
}

type transferMessage struct {
	TransferId      string `json:"transfer_id"`
	SrcBic          string `json:"src_bic"`
	SrcBan          string `json:"src_ban"`
	DestBic         string `json:"dest_bic"`
	DestBan         string `json:"dest_ban"`
	State           string `json:"state"`
	ClientId        string `json:"client_id"`
	ClientTimestamp string `json:"client_timestamp"`
	Amount          int64  `json:"amount"`
}

func (cluster *CartridgeCluster) InsertTransfer(_ *model.Transfer) error {
	return errors.New("implement me")
}

func (cluster *CartridgeCluster) DeleteTransfer(_ model.TransferId, _ uuid.UUID) error {
	return errors.New("implement me")
}

func (cluster *CartridgeCluster) SetTransferClient(
	clientID uuid.UUID,
	transferID model.TransferId,
) error {
	panic("implement me")
}

func (cluster *CartridgeCluster) FetchTransferClient(
	transferID model.TransferId,
) (*uuid.UUID, error) {
	panic("implement me")
}

func (cluster *CartridgeCluster) ClearTransferClient(
	transferID model.TransferId,
	clientID uuid.UUID,
) error {
	panic("implement me")
}

func (cluster *CartridgeCluster) SetTransferState(
	state string,
	transferID model.TransferId,
	clientID uuid.UUID,
) error {
	panic("implement me")
}

func (cluster *CartridgeCluster) FetchTransfer(
	transferID model.TransferId,
) (*model.Transfer, error) {
	panic("implement me")
}

func (cluster *CartridgeCluster) FetchDeadTransfers() ([]model.TransferId, error) {
	panic("implement me")
}

func (cluster *CartridgeCluster) UpdateBalance(
	balance *inf.Dec,
	bic string,
	ban string,
	transferID model.TransferId,
) error {
	panic("implement me")
}

func (cluster *CartridgeCluster) LockAccount(
	transferID model.TransferId,
	pendingAmount *inf.Dec,
	bic string,
	ban string,
) (*model.Account, error) {
	panic("implement me")
}

func (cluster *CartridgeCluster) UnlockAccount(
	bic string,
	ban string,
	transferID model.TransferId,
) error {
	panic("implement me")
}

// NewFoundationCluster - Создать подключение к cartridge и создать новые коллекции, если ещё не созданы.
func NewCartridgeCluster(dbURL string, poolSize uint64) (*CartridgeCluster, error) {
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

//nolint:golint,unused
func (cluster *CartridgeCluster) addSharding() error {
	return nil
}

// BootstrapDB - заполнить параметры настройки  и инициализировать ключ для хранения итогового баланса.
func (cluster *CartridgeCluster) BootstrapDB(count uint64, seed int) error {
	llog.Infof("Populating settings...")

	bodyTemplate := []byte(
		`{"count":` + fmt.Sprintf("%v", count) + `,"seed": ` + fmt.Sprintf("%v", seed) + `}`,
	)
	reqTemplate := fmt.Sprintf("%s/db/bootstrap", cluster.url)

	request, err := http.NewRequest(
		"POST", reqTemplate, bytes.NewBuffer(bodyTemplate), //nolint
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
	reqTemplate := fmt.Sprintf("%s/settings/fetch", cluster.url)
	request, err := http.NewRequest(
		"GET", reqTemplate, nil, //nolint
	)
	if err != nil {
		return Settings{}, merry.Prepend( //nolint
			err,
			"failed to create request to fetch settings in cartridge app",
		)
	}

	resp, err := cluster.client.Do(request)
	if err != nil {
		return Settings{}, merry.Prepend( //nolint
			err,
			"failed to make request to fetch settings in cartridge app",
		)
	}

	if resp.StatusCode != 200 {
		return Settings{}, merry.Prepend(err, "failed to fetch settings in cartridge app")
	}

	defer resp.Body.Close()

	var settings map[string]map[string]int

	if err = json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return Settings{}, merry.Prepend( //nolint
			err,
			"failed to decode settings from cartridge app response",
		)
	}

	return Settings{
		Count: settings["info"]["count"],
		Seed:  settings["info"]["seed"],
	}, nil
}

// InsertAccount - сохранить новый счет.
func (cluster *CartridgeCluster) InsertAccount(acc model.Account) (err error) {
	updAccount := account{ //nolint
		Bic:     acc.Bic,
		Ban:     acc.Ban,
		Balance: acc.Balance.UnscaledBig().Int64(),
		Found:   false,
	}

	accJSON, err := json.Marshal(updAccount)
	if err != nil {
		return merry.Prepend(
			err,
			"failed to marshal account to json to insert account in cartridge app",
		)
	}

	requestTemplate := fmt.Sprintf("%s/account/insert", cluster.url)

	request, err := http.NewRequest(
		http.MethodPost, requestTemplate, bytes.NewBuffer(accJSON),
	)
	if err != nil {
		return merry.Prepend(err, "failed to create request to insert account in cartridge app")
	}

	request.Header.Set("Content-Type", "application/json")

	resp, err := cluster.client.Do(request)
	if err != nil {
		return merry.Prepend(err, "failed to make request to insert account in cartridge app")
	}

	var response map[string]interface{}

	if resp.StatusCode != 201 {

		if resp.StatusCode == 409 || resp.StatusCode == 500 {

			if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
				unknownResponse, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					llog.Errorf(
						"failed to insert account in cartridge app because got unknown answer body: %v \n",
						err,
					)
				}

				llog.Errorf(
					"failed to decode json response from cartridge app, because got %v \n",
					string(unknownResponse),
				)
			}

			if response["error"] == "Account already exist" {
				return ErrDuplicateKey
			}
			if response["error"] == "Timeout exceeded" {
				return ErrTimeoutExceeded
			}
			return ErrInternalServerError
		}

		return merry.Errorf(
			"failed to insert account in cartridge app: %v %v",
			resp.StatusCode,
			response,
		)
	}

	defer resp.Body.Close()

	return nil
}

// FetchTotal - получить значение итогового баланса из Settings.
func (cluster *CartridgeCluster) FetchTotal() (*inf.Dec, error) {
	requestTemplate := fmt.Sprintf("%s/total_balance/fetch", cluster.url)
	request, err := http.NewRequest(
		http.MethodGet, requestTemplate, http.NoBody,
	)
	if err != nil {
		return nil, merry.Prepend(
			err,
			"failed to create request to fetch total balance in cartridge app",
		)
	}

	resp, err := cluster.client.Do(request)
	if err != nil {
		return nil, merry.Prepend(
			err,
			"failed to make request to fetch total balance in cartridge app",
		)
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
		return nil, merry.Prepend(
			err,
			"failed to convert balance string from cartridge app response",
		)
	}

	return inf.NewDec(balance, 0), nil
}

// PersistTotal - сохранить значение итогового баланса в settings.
func (cluster *CartridgeCluster) PersistTotal(total inf.Dec) error {
	bodyTemplate := []byte(`{"total":` + fmt.Sprintf("%v", total.UnscaledBig().Int64()) + `}`)
	requestTemplate := fmt.Sprintf("%s/total_balance/persist", cluster.url)

	request, err := http.NewRequest(
		http.MethodPost, requestTemplate, bytes.NewBuffer(bodyTemplate),
	)
	if err != nil {
		return merry.Prepend(
			err,
			"failed to create request to persist total balance in cartridge app",
		)
	}

	request.Header.Set("Content-Type", "application/json")

	resp, err := cluster.client.Do(request)
	if err != nil {
		return merry.Prepend(
			err,
			"failed to make request to persist total balance in cartridge app",
		)
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
	requestTemplate := fmt.Sprintf("%s/balance/check", cluster.url)
	request, err := http.NewRequest(
		http.MethodGet, requestTemplate, http.NoBody,
	)
	if err != nil {
		return nil, merry.Prepend(
			err,
			"failed to create request to calculate balance in cartridge app",
		)
	}

	resp, err := cluster.client.Do(request)
	if err != nil {
		return nil, merry.Prepend(
			err,
			"failed to make request to calculate balance in cartridge app",
		)
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
		return nil, merry.Prepend(
			err,
			"failed to convert balance string from cartridge app response",
		)
	}

	return inf.NewDec(balance, 0), nil
}

// MakeAtomicTransfer - выполнить операцию перевода и изменить балансы source и dest cчетов.
func (cluster *CartridgeCluster) MakeAtomicTransfer( //nolint
	transfer *model.Transfer,
	clientID uuid.UUID,
) error {
	// преобразуем в новую структуру для удобства обработки приложением. На логику принципиально не влияет.
	sendingTransfer := &transferMessage{
		TransferId:      transfer.Id.String(),
		SrcBic:          transfer.Acs[0].Bic,
		SrcBan:          transfer.Acs[0].Ban,
		DestBic:         transfer.Acs[1].Bic,
		DestBan:         transfer.Acs[1].Ban,
		State:           transfer.State,
		ClientId:        clientID.String(),
		ClientTimestamp: "",
		Amount:          0,
	}

	transferJson, err := json.Marshal(sendingTransfer)
	if err != nil {
		//nolint:golint,errcheck
		merry.Prepend(
			err,
			"failed to marshal transfer to json to make custom transfer in cartridge app",
		)
	}

	requestTemplate := fmt.Sprintf("%s/transfer/custom/create", cluster.url)

	request, err := http.NewRequest(
		http.MethodPost, requestTemplate, bytes.NewBuffer(transferJson),
	)
	if err != nil {
		return merry.Prepend(
			err,
			"failed to create request to make custom transfer in cartridge app",
		)
	}

	request.Header.Set("Content-Type", "application/json")

	resp, err := cluster.client.Do(request)
	if err != nil {
		return merry.Prepend(err, "failed to make request to make custom transfer in cartridge app")
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {

		if resp.StatusCode == 409 || resp.StatusCode == 500 {
			var response map[string]interface{}
			if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
				unknownResponse, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					llog.Errorf(
						"failed to make custom transfer in cartridge app because got unknown answer body: %v \n",
						err,
					)
				}

				llog.Errorf(
					"failed to decode json response from cartridge app, because got %v \n",
					string(unknownResponse),
				)
			}

			llog.Debugln(
				sendingTransfer.TransferId,
				sendingTransfer.ClientId,
				resp.StatusCode,
				response,
			)

			if response["error"] == "insufficient funds for transfer" {
				return ErrInsufficientFunds
			}
			if response["error"] == "Timeout exceeded" {
				return ErrTimeoutExceeded
			}
			return ErrInternalServerError
		}

		if resp.StatusCode == 404 {
			return ErrNoRows
		}

		return merry.Errorf(
			"failed to make custom transfer in cartridge app with response code: %v",
			resp.StatusCode,
		)
	}

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
