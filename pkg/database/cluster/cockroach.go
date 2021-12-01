/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package cluster

import (
	"context"
	"strconv"
	"strings"
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

func (cockroach *CockroachDatabase) InsertTransfer(transfer *model.Transfer) error {
	res, err := cockroach.pool.Exec(
		context.Background(),
		insertTransfer,
		transfer.Id,
		transfer.Acs[0].Bic,
		transfer.Acs[0].Ban,
		transfer.Acs[1].Bic,
		transfer.Acs[1].Ban,
		transfer.Amount.UnscaledBig().Int64(),
	)
	if res.RowsAffected() != 1 {
		return merry.Errorf("res.RowsAffected() is %v", res.RowsAffected())
	}
	if err != nil {
		return merry.Wrap(err)
	}

	return nil
}

func (cockroach *CockroachDatabase) DeleteTransfer(transferId model.TransferId, clientId uuid.UUID) error {
	res, err := cockroach.pool.Exec(context.Background(), deleteTransfer, transferId, clientId)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNoRows
		}
		return merry.Wrap(err)
	}
	if res.RowsAffected() != 1 {
		return merry.Errorf("res.RowsAffected() is %v", res.RowsAffected())
	}

	return nil
}

func (cockroach *CockroachDatabase) SetTransferClient(clientId uuid.UUID, transferId model.TransferId) error {
	res, err := cockroach.pool.Exec(context.Background(), setTransferClient, clientId, transferId)
	if err != nil {
		return merry.Wrap(err)
	}
	if res.RowsAffected() == 0 {
		return ErrNoRows
	}

	return nil
}

func (cockroach *CockroachDatabase) FetchTransferClient(transferId model.TransferId) (*uuid.UUID, error) {
	row := cockroach.pool.QueryRow(context.Background(), fetchTransferClient, transferId)

	var clientId uuid.UUID
	err := row.Scan(&clientId)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNoRows
		}
		return nil, merry.Wrap(err)
	}

	return &clientId, nil
}

func (cockroach *CockroachDatabase) ClearTransferClient(transferId model.TransferId, clientId uuid.UUID) error {
	res, err := cockroach.pool.Exec(context.Background(), clearTransferClient, transferId, clientId)
	if err != nil {
		return merry.Wrap(err)
	}
	if res.RowsAffected() != 1 {
		if err == pgx.ErrNoRows {
			return ErrNoRows
		}
	}

	return nil
}

func (cockroach *CockroachDatabase) SetTransferState(state string, transferId model.TransferId, clientId uuid.UUID) error {
	res, err := cockroach.pool.Exec(context.Background(), setTransferState, state, transferId, clientId)
	if err != nil {
		return merry.Wrap(err)
	}
	if res.RowsAffected() != 1 {
		return ErrNoRows
	}

	return nil
}

func (cockroach *CockroachDatabase) FetchTransfer(transferId model.TransferId) (*model.Transfer, error) {
	t := new(model.Transfer)
	t.InitEmptyTransfer(transferId)
	row := cockroach.pool.QueryRow(context.Background(), fetchTransfer, transferId)
	// Ignore possible error, we will retry
	var amount int64
	if err := row.Scan(&t.Acs[0].Bic, &t.Acs[0].Ban, &t.Acs[1].Bic,
		&t.Acs[1].Ban, &amount, &t.State); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNoRows
		}
		return nil, merry.Wrap(err)
	}
	t.Amount = inf.NewDec(amount, 0)
	return t, nil
}

func (cockroach *CockroachDatabase) FetchDeadTransfers() ([]model.TransferId, error) {
	rows, err := cockroach.pool.Query(context.Background(), fetchDeadTransfers)
	if err != nil {
		if err == pgx.ErrNoRows {
			return []model.TransferId{}, nil
		}

		return nil, merry.Wrap(err)
	}
	var transferIds []model.TransferId
	for rows.Next() {
		var tId model.TransferId
		err = rows.Scan(&tId)
		// probably should be ignored
		if err != nil {
			return nil, merry.Wrap(err)
		}
		transferIds = append(transferIds, tId)
	}

	return transferIds, nil
}

func (cockroach *CockroachDatabase) UpdateBalance(balance *inf.Dec, bic string, ban string, transferId model.TransferId) error {
	res, err := cockroach.pool.Exec(context.Background(), updateBalance, balance.UnscaledBig().Int64(), bic, ban, transferId)
	if err != nil {
		return merry.Wrap(err)
	}
	if res.RowsAffected() != 1 {
		return ErrNoRows
	}

	return nil
}

func (cockroach *CockroachDatabase) LockAccount(transferId model.TransferId, pendingAmount *inf.Dec,
	bic string, ban string) (*model.Account, error) {

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	row := cockroach.pool.QueryRow(ctx, lockAccount, transferId, pendingAmount.UnscaledBig().Int64(), bic, ban)

	var acc model.Account
	var resultBalance, resultPendingAmount int64
	err := row.Scan(&acc.Bic, &acc.Ban, &resultBalance, &acc.PendingTransfer, &resultPendingAmount)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrTimeoutExceeded
		}
		if err == pgx.ErrNoRows {
			return nil, ErrNoRows
		}
		return nil, merry.Prepend(err, "failed to scan locked account")
	}
	acc.Balance = inf.NewDec(resultBalance, 0)
	acc.PendingAmount = inf.NewDec(resultPendingAmount, 0)
	return &acc, nil
}

func (cockroach *CockroachDatabase) UnlockAccount(bic string, ban string, transferId model.TransferId) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	res, err := cockroach.pool.Exec(ctx, unlockAccount, bic, ban, transferId)
	if err != nil {
		if ctx.Err() != nil {
			return ErrTimeoutExceeded
		}
		return merry.Prepend(err, "failed to unlock account")
	}
	if res.RowsAffected() != 1 {
		return ErrNoRows
	}
	return nil
}

func NewCockroachCluster(dbURL string, connectionPoolSize int) (cluster *CockroachDatabase, err error) {
	llog.Infof("Establishing connection to cockroach on %v", dbURL)

	var poolConfig *pgxpool.Config
	if poolConfig, err = pgxpool.ParseConfig(dbURL); err != nil {
		err = merry.Prepend(err, "parse url parameter")
		return
	}

	if !strings.Contains(dbURL, "pool_max_conns") {
		poolConfig.MaxConns = int32(connectionPoolSize)
	}

	llog.Debugf("Connection pool size: %v", poolConfig.MaxConns)

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

const cockroachTimeoutSettings = 50 * time.Second

func (cockroach *CockroachDatabase) FetchSettings() (clusterSettings Settings, err error) {
	ctx, cancel := context.WithTimeout(cockroach.ctxt, cockroachTimeoutSettings)

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


const cockroachTxTimeout = 45 * time.Second

func (cockroach *CockroachDatabase) MakeAtomicTransfer(transfer *model.Transfer) error {
	ctx, cancel := context.WithTimeout(context.Background(), cockroachTxTimeout)
	defer cancel()

	// RepeatableRead is sufficient to provide consistent balance update even though
	// serialization anomalies are allowed that should not affect us (no dependable transaction, except obviously blocked rows)
	tx, err := cockroach.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead,
	})
	if err != nil {
		return merry.Prepend(err, "failed to acquire tx")
	}

	// Rollback is safe to call even if the tx is already closed, so if
	// the tx commits successfully, this is a no-op
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			llog.Errorf("failed to rollback transaction: '%v'", err)
			panic(ErrConsistencyViolation)
		}
	}()

	sourceAccount := transfer.Acs[0]
	destAccount := transfer.Acs[1]

	_, err = tx.Exec(
		ctx,
		insertTransfer,
		transfer.Id,
		sourceAccount.Bic,
		sourceAccount.Ban,
		destAccount.Bic,
		destAccount.Ban,
		transfer.Amount.UnscaledBig().Int64(),
	)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgerrcode.IsTransactionRollback(pgErr.Code) {
				return ErrTxRollback
			}
		}
		return merry.Prepend(err, "failed to insert transfer")
	}

	//	If we always withdraw money first deadlock may occur.
	//	Imagine we have concurrent txA (transfer X -> Y) and txB (transfer Y -> X).
	//	We will see the following timeline:
	//
	// 	1. txA locks account X to withdraw money	--- txB locks account Y to withdraw money
	//	2. txA tries to acquire lock on account Y	--- txB tries to acquire lock on accout X
	// 	3. PostgreSQL will wait deadlock_timeout (defaults to 1s) to check if we have deadlock (we do) and will abort txB.
	// 	4. Retry loop
	//
	// 	We have to select consistent lock order to avoid such troubles.
	// 	In that case we have the following timeline:
	//
	// 	1. txA locks account X to withdraw money	--- txB tries to acquire lock on account X to withdraw money
	//	2. txA locks account Y to top up money		--- txB waits for lock on X
	//  3. txA commits								--- txB acquires lock on X
	// 	4. 											--- txB acquires lock on Y
	// 	5.											--- txB commits
	//
	// 	TPS without lock order management is reduced drastically on default PostgreSQL configuration.
	if sourceAccount.AccountID() > destAccount.AccountID() {
		_, err = WithdrawMoney(ctx, tx, sourceAccount, *transfer)
		if err != nil {
			return merry.Prepend(err, "failed to withdraw money")
		}

		_, err = TopUpMoney(ctx, tx, destAccount, *transfer)
		if err != nil {
			return merry.Prepend(err, "failed to top up money")
		}
	} else {
		_, err = TopUpMoney(ctx, tx, destAccount, *transfer)
		if err != nil {
			return merry.Prepend(err, "failed to withdraw money")
		}
		_, err = WithdrawMoney(ctx, tx, sourceAccount, *transfer)
		if err != nil {
			return merry.Prepend(err, "failed to top up money")
		}
	}

	if err = tx.Commit(ctx); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgerrcode.IsTransactionRollback(pgErr.Code) {
				return ErrTxRollback
			}
		}
		return merry.Prepend(err, "failed to commit tx")
	}

	return nil
}

func (cockroach *CockroachDatabase) FetchAccounts() ([]model.Account, error) {
	rows, err := cockroach.pool.Query(context.Background(), fetchAccounts)
	if err != nil {
		return nil, merry.Prepend(err, "failed to fetch accounts")
	}
	var accs []model.Account
	for rows.Next() {
		var acc model.Account
		if err := rows.Scan(&acc); err != nil {
			return nil, merry.Prepend(err, "failed to scan account for FetchAccounts")
		}
		accs = append(accs, acc)
	}
	return accs, nil
}

func (cockroach *CockroachDatabase) FetchBalance(bic string, ban string) (*inf.Dec, *inf.Dec, error) {
	row := cockroach.pool.QueryRow(context.Background(), fetchBalance, bic, ban)
	var balance, pendingAmount inf.Dec
	err := row.Scan(&balance, &pendingAmount)
	if err != nil {
		return nil, nil, err
	}
	return &balance, &pendingAmount, nil
}

func (cockroach *CockroachDatabase) StartStatisticsCollect(_ time.Duration) (_ error) {
	llog.Warnln("stat metrics is not suppoerted now for cockroach")

	return
}
