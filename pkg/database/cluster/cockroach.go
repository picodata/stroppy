/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package cluster

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/picodata/stroppy/internal/model"

	llog "github.com/sirupsen/logrus"
	"gopkg.in/inf.v0"
)

type CockroachDatabase struct{}

func (cluster *CockroachDatabase) InsertTransfer(_ *model.Transfer) error {
	return errors.New("implement me")
}

func (cluster *CockroachDatabase) DeleteTransfer(_ model.TransferId, _ uuid.UUID) error {
	return errors.New("implement me")
}

func (cluster *CockroachDatabase) SetTransferClient(clientId uuid.UUID, transferId model.TransferId) error {
	panic("implement me")
}

func (cluster *CockroachDatabase) FetchTransferClient(transferId model.TransferId) (*uuid.UUID, error) {
	panic("implement me")
}

func (cluster *CockroachDatabase) ClearTransferClient(transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cluster *CockroachDatabase) SetTransferState(state string, transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cluster *CockroachDatabase) FetchTransfer(transferId model.TransferId) (*model.Transfer, error) {
	panic("implement me")
}

func (cluster *CockroachDatabase) FetchDeadTransfers() ([]model.TransferId, error) {
	panic("implement me")
}

func (cluster *CockroachDatabase) UpdateBalance(balance *inf.Dec, bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

func (cluster *CockroachDatabase) LockAccount(transferId model.TransferId, pendingAmount *inf.Dec, bic string, ban string) (*model.Account, error) {
	panic("implement me")
}

func (cluster *CockroachDatabase) UnlockAccount(bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

func NewCocroachCluster(dbURL string) (*CockroachDatabase, error) {
	llog.Infof("Establishing connection to cockroach on %v", dbURL)

	return nil, errors.New("not yet implemented")
}

func (cluster *CockroachDatabase) BootstrapDB(count int, seed int) error {
	return nil
}

func (cluster *CockroachDatabase) GetClusterType() DBClusterType {
	return CockroachClusterType
}

func (cluster *CockroachDatabase) FetchSettings() (s Settings, err error) {
	return
}

func (cluster *CockroachDatabase) InsertAccount(acc model.Account) error {
	return nil
}

func (cluster *CockroachDatabase) FetchTotal() (*inf.Dec, error) {
	return nil, nil
}

func (cluster *CockroachDatabase) PersistTotal(_ inf.Dec) error {
	return nil
}

func (cluster *CockroachDatabase) CheckBalance() (*inf.Dec, error) {
	return nil, nil
}

func (cluster *CockroachDatabase) MakeAtomicTransfer(_ *model.Transfer) error {
	return errors.New("not yet implemented")
}

func (cluster *CockroachDatabase) FetchAccounts() ([]model.Account, error) {
	return nil, errors.New("not yet implemented")
}

func (cluster *CockroachDatabase) FetchBalance(bic string, ban string) (*inf.Dec, *inf.Dec, error) {
	return nil, nil, errors.New("not yet implemented")
}

func (cluster *CockroachDatabase) StartStatisticsCollect(statInterval time.Duration) error {
	return nil
}
