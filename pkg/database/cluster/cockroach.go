/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package cluster

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/ansel1/merry"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"github.com/google/uuid"

	"gitlab.com/picodata/stroppy/internal/model"

	llog "github.com/sirupsen/logrus"
	"gopkg.in/inf.v0"
)

type CockroachDatabase struct {
	pool *pgxpool.Pool
	ctxt context.Context
}

func (cockroach *CockroachDatabase) InsertTransfer(_ *model.Transfer) error {
	return errors.New("implement me")
}

func (cockroach *CockroachDatabase) DeleteTransfer(_ model.TransferId, _ uuid.UUID) error {
	return errors.New("implement me")
}

func (cockroach *CockroachDatabase) SetTransferClient(clientId uuid.UUID, transferId model.TransferId) error {
	panic("implement me")
}

func (cockroach *CockroachDatabase) FetchTransferClient(transferId model.TransferId) (*uuid.UUID, error) {
	panic("implement me")
}

func (cockroach *CockroachDatabase) ClearTransferClient(transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cockroach *CockroachDatabase) SetTransferState(state string, transferId model.TransferId, clientId uuid.UUID) error {
	panic("implement me")
}

func (cockroach *CockroachDatabase) FetchTransfer(transferId model.TransferId) (*model.Transfer, error) {
	panic("implement me")
}

func (cockroach *CockroachDatabase) FetchDeadTransfers() ([]model.TransferId, error) {
	panic("implement me")
}

func (cockroach *CockroachDatabase) UpdateBalance(balance *inf.Dec, bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

func (cockroach *CockroachDatabase) LockAccount(transferId model.TransferId, pendingAmount *inf.Dec, bic string, ban string) (*model.Account, error) {
	panic("implement me")
}

func (cockroach *CockroachDatabase) UnlockAccount(bic string, ban string, transferId model.TransferId) error {
	panic("implement me")
}

func NewCockroachCluster(dbURL string) (cluster *CockroachDatabase, err error) {
	llog.Infof("Establishing connection to cockroach on %v", dbURL)

	var poolConfig *pgxpool.Config
	if poolConfig, err = pgxpool.ParseConfig(dbURL); err != nil {
		err = merry.Prepend(err, "parse url parameter")
		return
	}

	ctxt := context.Background()

	var pgPool *pgxpool.Pool
	if pgPool, err = pgxpool.ConnectConfig(ctxt, poolConfig); err != nil {
		err = merry.Prepend(err, "connection")
		return
	}

	cluster = &CockroachDatabase{
		pool: pgPool,
		ctxt: ctxt,
	}
	return
}

func (cockroach *CockroachDatabase) BootstrapDB(count int, seed int) (err error) {
	llog.Infof("Bootstrapping cluster...")
	if _, err = cockroach.pool.Exec(cockroach.ctxt, bootstrapScript); err != nil {
		return merry.Prepend(err, "failed to execute bootstrap script")
	}

	llog.Infof("Loading settings...")
	_, err = cockroach.pool.Exec(cockroach.ctxt, insertSetting, "count", strconv.Itoa(count))
	if err != nil {
		return merry.Prepend(err, "failed to load count setting")
	}

	_, err = cockroach.pool.Exec(cockroach.ctxt, insertSetting, "seed", strconv.Itoa(seed))
	if err != nil {
		return merry.Prepend(err, "failed to load seed setting")
	}

	return
}

func (cockroach *CockroachDatabase) GetClusterType() DBClusterType {
	return CockroachClusterType
}

func (cockroach *CockroachDatabase) FetchSettings() (clusterSettings Settings, err error) {
	ctx, cancel := context.WithTimeout(cockroach.ctxt, timeOutSettings*time.Second)
	defer cancel()

	var rows pgx.Rows
	rows, err = cockroach.pool.Query(ctx, fetchSettings)
	if err != nil {
		return Settings{
			Count: 0,
			Seed:  0,
		}, merry.Prepend(err, "failed to fetch settings")
	}

	var fetchSettings []string
	for rows.Next() {
		var clusterSetting string
		if err := rows.Scan(&clusterSetting); err != nil {
			return clusterSettings, merry.Prepend(err, "failed to scan setting for FetchSettings")
		}
		fetchSettings = append(fetchSettings, clusterSetting)
	}

	clusterSettings.Count, err = strconv.Atoi(fetchSettings[0])
	if err != nil {
		return Settings{
				Count: 0,
				Seed:  0,
			},
			merry.Prepend(err, "failed to get count setting for FetchSettings")
	}

	clusterSettings.Seed, err = strconv.Atoi(fetchSettings[1])
	if err != nil {
		return Settings{
				Count: 0,
				Seed:  0,
			},
			merry.Prepend(err, "failed to get seed setting for FetchSettings")
	}

	return
}

func (cockroach *CockroachDatabase) InsertAccount(acc model.Account) (err error) {
	_, err = cockroach.pool.Exec(cockroach.ctxt, upsertAccount, acc.Bic, acc.Ban, acc.Balance.UnscaledBig().Int64())
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgErr.Code == pgerrcode.UniqueViolation {
				return merry.Wrap(ErrDuplicateKey)
			}
		}

		return merry.Wrap(err)
	}

	return
}

func (cockroach *CockroachDatabase) FetchTotal() (*inf.Dec, error) {
	row := cockroach.pool.QueryRow(context.Background(), fetchTotal)

	var amount inf.Dec
	if err := row.Scan(&amount); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNoRows
		}
		return nil, merry.Wrap(err)
	}

	return &amount, nil
}

func (cockroach *CockroachDatabase) PersistTotal(total inf.Dec) error {
	res, err := cockroach.pool.Exec(context.Background(), persistTotal, total.UnscaledBig().Int64())
	if err != nil {
		return merry.Wrap(err)
	}
	if res.RowsAffected() != 1 {
		return merry.Errorf("PersistTotal() res.RowsAffected is %v", res.RowsAffected())
	}

	return nil
}

func (cockroach *CockroachDatabase) CheckBalance() (*inf.Dec, error) {
	row := cockroach.pool.QueryRow(context.Background(), checkBalance)
	var totalBalance int64
	err := row.Scan(&totalBalance)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNoRows
		}
		return nil, merry.Wrap(err)
	}

	return inf.NewDec(totalBalance, 0), nil
}

func (cockroach *CockroachDatabase) MakeAtomicTransfer(_ *model.Transfer) error {
	return errors.New("not yet implemented")
}

func (cockroach *CockroachDatabase) FetchAccounts() ([]model.Account, error) {
	return nil, errors.New("not yet implemented")
}

func (cockroach *CockroachDatabase) FetchBalance(bic string, ban string) (*inf.Dec, *inf.Dec, error) {
	return nil, nil, errors.New("not yet implemented")
}

func (cockroach *CockroachDatabase) StartStatisticsCollect(_ time.Duration) (_ error) {
	llog.Warnln("stat metrics is not suppoerted now for cockroach")

	return
}
