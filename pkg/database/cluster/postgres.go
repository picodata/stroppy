/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package cluster

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gitlab.com/picodata/stroppy/internal/model"

	"github.com/ansel1/merry"
	"github.com/google/uuid"
	llog "github.com/sirupsen/logrus"
	"gopkg.in/inf.v0"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type PostgresCluster struct {
	pool *pgxpool.Pool
}

func NewPostgresCluster(dbURL string, connectionPoolCount int) (*PostgresCluster, error) {
	llog.Infof("Establishing connection to pg on %v", dbURL)

	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, merry.Wrap(err)
	}

	if !strings.Contains(dbURL, "pool_max_conns") {
		poolConfig.MaxConns = int32(connectionPoolCount)
	}

	llog.Debugf("Connection pool size: %v", poolConfig.MaxConns)

	pgPool, err := pgxpool.ConnectConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, merry.Wrap(err)
	}

	return &PostgresCluster{
		pgPool,
	}, nil
}

func (*PostgresCluster) GetClusterType() DBClusterType {
	return PostgresClusterType
}

func (pgc *PostgresCluster) BootstrapDB(count int, seed int) error {
	llog.Infof("Creating the tables...")

	_, err := pgc.pool.Exec(context.Background(), bootstrapScript)
	if err != nil {
		return merry.Prepend(err, "failed to execute bootstrap script")
	}

	llog.Infof("Populating settings...")

	_, err = pgc.pool.Exec(context.Background(), insertSetting, "count", strconv.Itoa(count))
	if err != nil {
		return merry.Prepend(err, "failed to populate settings")
	}

	_, err = pgc.pool.Exec(context.Background(), insertSetting, "seed", strconv.Itoa(seed))
	if err != nil {
		return merry.Prepend(err, "failed to save seed")
	}

	return nil
}

func (pgc *PostgresCluster) FetchSettings() (Settings, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeOutSettings*time.Second)

	defer cancel()

	rows, err := pgc.pool.Query(ctx, fetchSettings)
	if err != nil {
		return Settings{
			Count: 0,
			Seed:  0,
		}, merry.Prepend(err, "failed to fetch settings")
	}

	var clusterSettings Settings

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

	return clusterSettings, nil
}

func (pgc *PostgresCluster) InsertAccount(acc model.Account) error {
	_, err := pgc.pool.Exec(context.Background(), upsertAccount, acc.Bic, acc.Ban, acc.Balance.UnscaledBig().Int64())
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgErr.Code == pgerrcode.UniqueViolation {
				return merry.Wrap(ErrDuplicateKey)
			}
		}

		return merry.Wrap(err)
	}

	return nil
}

func (pgc *PostgresCluster) FetchAccounts() ([]model.Account, error) {
	rows, err := pgc.pool.Query(context.Background(), `SELECT bic, ban, balance FROM account;`)
	if err != nil {
		return nil, merry.Prepend(err, "failed to fetch accounts")
	}

	return accRowsToSlice(rows)
}

func (pgc *PostgresCluster) FetchTotal() (*inf.Dec, error) {
	row := pgc.pool.QueryRow(context.Background(), fetchTotal)

	var amount inf.Dec

	err := row.Scan(&amount)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNoRows
		}

		return nil, merry.Wrap(err)
	}

	return &amount, nil
}

func (pgc *PostgresCluster) PersistTotal(total inf.Dec) error {
	res, err := pgc.pool.Exec(context.Background(), persistTotal, total.UnscaledBig().Int64())
	if err != nil {
		return merry.Wrap(err)
	}

	if res.RowsAffected() != 1 {
		return merry.Errorf("PersistTotal() res.RowsAffected is %v", res.RowsAffected())
	}

	return nil
}

func (pgc *PostgresCluster) CheckBalance() (*inf.Dec, error) {
	row := pgc.pool.QueryRow(context.Background(), checkBalance)

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

func (pgc *PostgresCluster) InsertTransfer(transfer *model.Transfer) error {
	res, err := pgc.pool.Exec(
		context.Background(),
		insertTransfer,
		transfer.ID,
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

func (pgc *PostgresCluster) SetTransferState(state string, transferID model.TransferID, clientID uuid.UUID) error {
	res, err := pgc.pool.Exec(context.Background(), setTransferState, state, transferID, clientID)
	if err != nil {
		return merry.Wrap(err)
	}

	if res.RowsAffected() != 1 {
		return ErrNoRows
	}

	return nil
}

func (pgc *PostgresCluster) SetTransferClient(clientID uuid.UUID, transferID model.TransferID) error {
	res, err := pgc.pool.Exec(context.Background(), setTransferClient, clientID, transferID)
	if err != nil {
		return merry.Wrap(err)
	}

	if res.RowsAffected() == 0 {
		return ErrNoRows
	}

	return nil
}

func (pgc *PostgresCluster) ClearTransferClient(transferID model.TransferID, clientID uuid.UUID) error {
	res, err := pgc.pool.Exec(context.Background(), clearTransferClient, transferID, clientID)
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

func (pgc *PostgresCluster) DeleteTransfer(transferID model.TransferID, clientID uuid.UUID) error {
	res, err := pgc.pool.Exec(context.Background(), deleteTransfer, transferID, clientID)
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

func (pgc *PostgresCluster) FetchTransfer(transferID model.TransferID) (*model.Transfer, error) {
	t := new(model.Transfer)
	t.InitEmptyTransfer(transferID)
	row := pgc.pool.QueryRow(context.Background(), fetchTransfer, transferID)
	// Ignore possible error, we will retry
	var amount int64

	if err := row.Scan(&t.Acs[0].Bic, &t.Acs[0].Ban, &t.Acs[1].Bic, &t.Acs[1].Ban, &amount, &t.State); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNoRows
		}

		return nil, merry.Wrap(err)
	}

	t.Amount = inf.NewDec(amount, 0)

	return t, nil
}

func (pgc *PostgresCluster) FetchTransferClient(transferID model.TransferID) (*uuid.UUID, error) {
	row := pgc.pool.QueryRow(context.Background(), fetchTransferClient, transferID)

	var clientID uuid.UUID

	err := row.Scan(&clientID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNoRows
		}

		return nil, merry.Wrap(err)
	}

	return &clientID, nil
}

func (pgc *PostgresCluster) LockAccount(transferID model.TransferID, pendingAmount *inf.Dec, bic string, ban string) (*model.Account, error) {
	panic("implement me")
}

func (pgc *PostgresCluster) UnlockAccount(bic string, ban string, transferID model.TransferID) error {
	panic("implement me")
}

func (pgc *PostgresCluster) UpdateBalance(balance *inf.Dec, bic string, ban string, transferID model.TransferID) error {
	panic("implement me")
}

func (pgc *PostgresCluster) FetchBalance(bic string, ban string) (*inf.Dec, *inf.Dec, error) {
	row := pgc.pool.QueryRow(context.Background(), fetchBalance, bic, ban)

	var balance, pendingAmount inf.Dec

	err := row.Scan(&balance, &pendingAmount)
	if err != nil {
		return nil, nil, err
	}

	return &balance, &pendingAmount, nil
}

func (pgc *PostgresCluster) FetchDeadTransfers() ([]model.TransferID, error) {
	rows, err := pgc.pool.Query(context.Background(), fetchDeadTransfers)
	if err != nil {
		if err == pgx.ErrNoRows {
			return []model.TransferID{}, nil
		}

		return nil, merry.Wrap(err)
	}

	var transferIds []model.TransferID

	for rows.Next() {
		var id model.TransferID
		err = rows.Scan(&id)
		// probably should be ignored
		if err != nil {
			return nil, merry.Wrap(err)
		}

		transferIds = append(transferIds, id)
	}

	return transferIds, nil
}

func WithdrawMoney(ctx context.Context, tx pgx.Tx, acc model.Account, transfer model.Transfer) error {
	// update balance
	row := tx.QueryRow(
		ctx,
		`
	UPDATE account SET balance = balance - $1 WHERE bic = $2 and ban = $3 RETURNING balance, (
		select balance from account where bic = $2 and ban = $3 -- that works only because of READ_COMMITTED isolation level
	  ) as old_balance, now();
	`,
		transfer.Amount.UnscaledBig().Int64(),
		acc.Bic,
		acc.Ban,
	)

	var oldBalance int64

	var newBalance int64

	var operationTime time.Time

	if err := row.Scan(&newBalance, &oldBalance, &operationTime); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgerrcode.IsTransactionRollback(pgErr.Code) {
				return ErrTxRollback
			}
		}
		// failed to find account
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoRows
		}

		return merry.Prepend(err, "failed to update first balance")
	}

	if newBalance < 0 {
		return ErrInsufficientFunds
	}

	return nil
}

func TopUpMoney(ctx context.Context, tx pgx.Tx, acc model.Account, transfer model.Transfer) error {
	// update balance
	row := tx.QueryRow(
		ctx,
		`
	UPDATE account SET balance = balance + $1 WHERE bic = $2 and ban = $3 RETURNING balance, (
		select balance from account where bic = $2 and ban = $3 -- that works only because of READ_COMMITTED isolation level
	  ) as old_balance, now();
	`,
		transfer.Amount.UnscaledBig().Int64(),
		acc.Bic,
		acc.Ban,
	)

	var oldBalance int64

	var newBalance int64

	var operationTime time.Time

	if err := row.Scan(&newBalance, &oldBalance, &operationTime); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgerrcode.IsTransactionRollback(pgErr.Code) {
				return ErrTxRollback
			}
		}
		// failed to find account
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoRows
		}

		return merry.Prepend(err, "failed to update first balance")
	}

	return nil
}

// MakeAtomicTransfer inserts new transfer (should be used as history in the future) and
// update corresponding balances in a single SQL transaction.
func (pgc *PostgresCluster) MakeAtomicTransfer(transfer *model.Transfer, clientID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), txTimeout)
	defer cancel()

	// RepeateableRead is sufficient to provide consistent balance update even though
	// serialization anomalies are allowed that should not affect us (no dependable transaction, except obviously blocked rows).
	tx, err := pgc.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
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
		transfer.ID,
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
		err = WithdrawMoney(ctx, tx, sourceAccount, *transfer)
		if err != nil {
			return merry.Prepend(err, "failed to withdraw money")
		}

		err = TopUpMoney(ctx, tx, destAccount, *transfer)
		if err != nil {
			return merry.Prepend(err, "failed to top up money")
		}
	} else {
		err = TopUpMoney(ctx, tx, destAccount, *transfer)
		if err != nil {
			return merry.Prepend(err, "failed to withdraw money")
		}

		err = WithdrawMoney(ctx, tx, sourceAccount, *transfer)
		if err != nil {
			return merry.Prepend(err, "failed to top up money")
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgerrcode.IsTransactionRollback(pgErr.Code) {
				return ErrTxRollback
			}
		}

		return merry.Prepend(err, "failed to commit tx")
	}

	merry.Prepend(err, "failed to insert new history item")

	return nil
}

func (pgc *PostgresCluster) StartStatisticsCollect(_ time.Duration) error {
	llog.Debugln("statistic for postgres not supported, watch grafana metrics, please")

	return nil
}
