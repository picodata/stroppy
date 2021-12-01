/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package cluster

import (
	"context"
	"errors"
	"strconv"
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

func NewPostgresCluster(dbURL string) (*PostgresCluster, error) {
	llog.Infof("Establishing connection to pg on %v", dbURL)

	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, merry.Wrap(err)
	}

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

func (self *PostgresCluster) BootstrapDB(count int, seed int) error {
	llog.Infof("Creating the tables...")
	_, err := self.pool.Exec(context.Background(), bootstrapScript)
	if err != nil {
		return merry.Prepend(err, "failed to execute bootstrap script")
	}
	llog.Infof("Populating settings...")
	_, err = self.pool.Exec(context.Background(), insertSetting, "count", strconv.Itoa(count))
	if err != nil {
		return merry.Prepend(err, "failed to populate settings")
	}

	_, err = self.pool.Exec(context.Background(), insertSetting, "seed", strconv.Itoa(seed))
	if err != nil {
		return merry.Prepend(err, "failed to save seed")
	}

	return nil
}

func (self *PostgresCluster) FetchSettings() (Settings, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeOutSettings*time.Second)
	defer cancel()
	rows, err := self.pool.Query(ctx, fetchSettings)
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

func (self *PostgresCluster) InsertAccount(acc model.Account) error {
	_, err := self.pool.Exec(context.Background(), upsertAccount, acc.Bic, acc.Ban, acc.Balance.UnscaledBig().Int64())
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

func (self *PostgresCluster) FetchAccounts() ([]model.Account, error) {
	rows, err := self.pool.Query(context.Background(), fetchAccounts)
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

func (self *PostgresCluster) FetchTotal() (*inf.Dec, error) {
	row := self.pool.QueryRow(context.Background(), fetchTotal)

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

func (self *PostgresCluster) PersistTotal(total inf.Dec) error {
	res, err := self.pool.Exec(context.Background(), persistTotal, total.UnscaledBig().Int64())
	if err != nil {
		return merry.Wrap(err)
	}
	if res.RowsAffected() != 1 {
		return merry.Errorf("PersistTotal() res.RowsAffected is %v", res.RowsAffected())
	}

	return nil
}

func (self *PostgresCluster) CheckBalance() (*inf.Dec, error) {
	row := self.pool.QueryRow(context.Background(), checkBalance)
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

func (self *PostgresCluster) InsertTransfer(transfer *model.Transfer) error {
	res, err := self.pool.Exec(
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

func (self *PostgresCluster) SetTransferState(state string, transferId model.TransferId, clientId uuid.UUID) error {
	res, err := self.pool.Exec(context.Background(), setTransferState, state, transferId, clientId)
	if err != nil {
		return merry.Wrap(err)
	}
	if res.RowsAffected() != 1 {
		return ErrNoRows
	}

	return nil
}

func (self *PostgresCluster) SetTransferClient(clientId uuid.UUID, transferId model.TransferId) error {
	res, err := self.pool.Exec(context.Background(), setTransferClient, clientId, transferId)
	if err != nil {
		return merry.Wrap(err)
	}
	if res.RowsAffected() == 0 {
		return ErrNoRows
	}

	return nil
}

func (self *PostgresCluster) ClearTransferClient(transferId model.TransferId, clientId uuid.UUID) error {
	res, err := self.pool.Exec(context.Background(), clearTransferClient, transferId, clientId)
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

func (self *PostgresCluster) DeleteTransfer(transferId model.TransferId, clientId uuid.UUID) error {
	res, err := self.pool.Exec(context.Background(), deleteTransfer, transferId, clientId)
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

func (self *PostgresCluster) FetchTransfer(transferId model.TransferId) (*model.Transfer, error) {
	t := new(model.Transfer)
	t.InitEmptyTransfer(transferId)
	row := self.pool.QueryRow(context.Background(), fetchTransfer, transferId)
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

func (self *PostgresCluster) FetchTransferClient(transferId model.TransferId) (*uuid.UUID, error) {
	row := self.pool.QueryRow(context.Background(), fetchTransferClient, transferId)

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

func (self *PostgresCluster) LockAccount(transferId model.TransferId, pendingAmount *inf.Dec, bic string, ban string) (*model.Account, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	row := self.pool.QueryRow(ctx, lockAccount, transferId, pendingAmount.UnscaledBig().Int64(), bic, ban)

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

func (self *PostgresCluster) UnlockAccount(bic string, ban string, transferId model.TransferId) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	res, err := self.pool.Exec(ctx, unlockAccount, bic, ban, transferId)
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

func (self *PostgresCluster) UpdateBalance(balance *inf.Dec, bic string, ban string, transferId model.TransferId) error {
	res, err := self.pool.Exec(context.Background(), updateBalance, balance.UnscaledBig().Int64(), bic, ban, transferId)
	if err != nil {
		return merry.Wrap(err)
	}
	if res.RowsAffected() != 1 {
		return ErrNoRows
	}

	return nil
}

func (self *PostgresCluster) FetchBalance(bic string, ban string) (*inf.Dec, *inf.Dec, error) {
	row := self.pool.QueryRow(context.Background(), fetchBalance, bic, ban)
	var balance, pendingAmount inf.Dec
	err := row.Scan(&balance, &pendingAmount)
	if err != nil {
		return nil, nil, err
	}
	return &balance, &pendingAmount, nil
}

func (self *PostgresCluster) FetchDeadTransfers() ([]model.TransferId, error) {
	rows, err := self.pool.Query(context.Background(), fetchDeadTransfers)
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

func WithdrawMoney(
	ctx context.Context,
	tx pgx.Tx,
	acc model.Account,
	transfer model.Transfer,
) (*model.HistoryItem, error) {
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
		//nolint:errorlint
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgerrcode.IsTransactionRollback(pgErr.Code) {
				return nil, ErrTxRollback
			}
		}
		// failed to find account
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoRows
		}
		return nil, merry.Prepend(err, "failed to update first balance")
	}
	if newBalance < 0 {
		return nil, ErrInsufficientFunds
	}
	newBalanceDec := inf.NewDec(newBalance, 0)
	oldBalanceDec := inf.NewDec(oldBalance, 0)

	history := model.NewHistoryItem(
		transfer.Id,
		acc.Bic,
		acc.Ban,
		oldBalanceDec,
		newBalanceDec,
		operationTime,
	)

	return &history, nil
}

func TopUpMoney(
	ctx context.Context,
	tx pgx.Tx,
	acc model.Account,
	transfer model.Transfer,
) (*model.HistoryItem, error) {
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
		//nolint:errorlint
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgerrcode.IsTransactionRollback(pgErr.Code) {
				return nil, ErrTxRollback
			}
		}
		// failed to find account
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoRows
		}
		return nil, merry.Prepend(err, "failed to update first balance")
	}
	newBalanceDec := inf.NewDec(newBalance, 0)
	oldBalanceDec := inf.NewDec(oldBalance, 0)

	history := model.NewHistoryItem(
		transfer.Id,
		acc.Bic,
		acc.Ban,
		oldBalanceDec,
		newBalanceDec,
		operationTime,
	)

	return &history, nil
}

// MakeAtomicTransfer inserts new transfer (should be used as history in the future) and
// update corresponding balances in a single SQL transaction
func (self *PostgresCluster) MakeAtomicTransfer(transfer *model.Transfer) error {
	ctx, cancel := context.WithTimeout(context.Background(), txTimeout)
	defer cancel()

	// RepeateableRead is sufficient to provide consistent balance update even though
	// serialization anomalies are allowed that should not affect us (no dependable transaction, except obviously blocked rows)
	tx, err := self.pool.BeginTx(ctx, pgx.TxOptions{
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
	var sourceHistoryItem, destHistoryItem *model.HistoryItem
	//nolint:nestif
	if sourceAccount.AccountID() > destAccount.AccountID() {
		sourceHistoryItem, err = WithdrawMoney(ctx, tx, sourceAccount, *transfer)
		if err != nil {
			return merry.Prepend(err, "failed to withdraw money")
		}

		destHistoryItem, err = TopUpMoney(ctx, tx, destAccount, *transfer)
		if err != nil {
			return merry.Prepend(err, "failed to top up money")
		}
	} else {
		destHistoryItem, err = TopUpMoney(ctx, tx, destAccount, *transfer)
		if err != nil {
			return merry.Prepend(err, "failed to withdraw money")
		}

		sourceHistoryItem, err = WithdrawMoney(ctx, tx, sourceAccount, *transfer)
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

	_, err = self.pool.Exec(
		ctx,
		`
		INSERT INTO history (
			id, transfer_id, account_bic, account_ban, old_balance, new_balance, operation_time
		) VALUES ($1, $2, $3, $4, $5, $6, $7);
		`,
		sourceHistoryItem.ID,
		sourceHistoryItem.TransferID,
		sourceHistoryItem.AccountBic,
		sourceHistoryItem.AccountBan,
		sourceHistoryItem.OldBalance.UnscaledBig().Uint64(),
		sourceHistoryItem.NewBalance.UnscaledBig().Uint64(),
		sourceHistoryItem.OperationTime,
	)
	if err != nil {
		return merry.Prepend(err, "failed to insert new history item")
	}

	_, err = self.pool.Exec(
		ctx,
		`
		INSERT INTO history (
			id, transfer_id, account_bic, account_ban, old_balance, new_balance, operation_time
		) VALUES ($1, $2, $3, $4, $5, $6, $7);
		`,
		destHistoryItem.ID,
		destHistoryItem.TransferID,
		destHistoryItem.AccountBic,
		destHistoryItem.AccountBan,
		destHistoryItem.OldBalance.UnscaledBig().Uint64(),
		destHistoryItem.NewBalance.UnscaledBig().Uint64(),
		destHistoryItem.OperationTime,
	)
	if err != nil {
		return merry.Prepend(err, "failed to insert new history item")
	}

	return nil
}

func (self *PostgresCluster) StartStatisticsCollect(_ time.Duration) error {
	llog.Debugln("statistic for postgres not supported, watch grafana metrics, please")
	return nil
}
