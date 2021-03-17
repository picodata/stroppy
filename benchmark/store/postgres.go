package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ansel1/merry"
	"github.com/google/uuid"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/benchmark/stroppy/model"
	"gopkg.in/inf.v0"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

const txTimeout = 5 * time.Second

type PostgresCluster struct {
	pool *pgxpool.Pool
}

func NewPostgresCluster(dbUrl string) (*PostgresCluster, error) {
	llog.Infof("Establishing connection to pg on %v", dbUrl)

	poolConfig, err := pgxpool.ParseConfig(dbUrl)
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

const bootstrapScript = `
CREATE TABLE IF NOT EXISTS setting (
     key TEXT PRIMARY KEY, -- arbitrary setting name
     value TEXT -- arbitrary setting value
);
TRUNCATE setting;

CREATE TABLE IF NOT EXISTS account (
	bic TEXT, -- bank identifier code
	ban TEXT, -- bank account number within the bank
	balance DECIMAL, -- account balance
	pending_transfer UUID, -- will be used later
	pending_amount DECIMAL, -- will be used later
	PRIMARY KEY(bic, ban)
);
TRUNCATE account;

CREATE TABLE IF NOT EXISTS transfer (
    transfer_id UUID PRIMARY KEY, -- transfers UUID
    src_bic TEXT, -- source bank identification code
    src_ban TEXT, -- source bank account number
    dst_bic TEXT, -- destination bank identification code
    dst_ban TEXT, -- destination bank account number
    amount DECIMAL, -- transfer amount
    state TEXT, -- 'new', 'locked', 'complete'
    client_id UUID, -- the client performing the transfer
	client_timestamp TIMESTAMP -- timestamp to implement TTL
);
TRUNCATE transfer;

CREATE TABLE IF NOT EXISTS checksum (
	name TEXT PRIMARY KEY,
	amount DECIMAL
);
TRUNCATE checksum;
`

const insertSetting = `
INSERT INTO setting (key, value) VALUES ($1, $2);
`

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

func (self *PostgresCluster) FetchSettings() (ClusterSettings, error) {
	return ClusterSettings{
		Count: 100,
		Seed:  100,
	}, nil
}

const upsertAccount = `
	INSERT INTO account (bic, ban, balance, pending_amount) ` +
	`VALUES ($1, $2, $3, 0) ON CONFLICT (bic, ban) DO UPDATE ` +
	`SET balance = excluded.balance, pending_amount = 0;
`

func (self *PostgresCluster) InsertAccount(acc model.Account) error {
	res, err := self.pool.Exec(context.Background(), upsertAccount, acc.Bic, acc.Ban, acc.Balance.UnscaledBig().Int64())
	if err != nil {
		return merry.Wrap(err)
	}

	if res.RowsAffected() != 1 {
		return merry.New("insertAccount res.RowsAffected() != 1")
	}

	return nil
}

const fetchAccounts = `
SELECT * FROM account
`

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

const fetchTotal = `
SELECT amount FROM checksum WHERE name = 'total;'
`

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

const persistTotal = `
INSERT INTO checksum (name, amount) VALUES('total', $1) ON CONFLICT (name) DO UPDATE SET amount = excluded.amount;
`

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

const checkBalance = `
SELECT SUM(balance) FROM account
`

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

// Client id has to be updated separately to let it expire
const insertTransfer = `
INSERT INTO transfer
  (transfer_id, src_bic, src_ban, dst_bic, dst_ban, amount, state)
  VALUES ($1, $2, $3, $4, $5, $6, 'complete')
`

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

const setTransferState = `
UPDATE transfer
  SET state = $1
  WHERE transfer_id = $2
  AND amount IS NOT NULL AND client_id = $3 AND client_timestamp > now() - interval'30 second'
`

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

const setTransferClient = `
UPDATE transfer
  SET client_id = $1, client_timestamp = now()
  WHERE transfer_id = $2
  AND amount IS NOT NULL
`

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

const clearTransferClient = `
UPDATE transfer
  SET client_id = NULL
  WHERE transfer_id = $1
  AND amount IS NOT NULL AND client_id = $2
`

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

const deleteTransfer = `
DELETE FROM transfer
  WHERE transfer_id = $1
  AND client_id = $2 AND client_timestamp > now() - interval '30 second'
`

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

const fetchTransfer = `
SELECT src_bic, src_ban, dst_bic, dst_ban, amount, state
  FROM transfer
  WHERE transfer_id = $1
`

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

const fetchTransferClient = `
SELECT client_id
  FROM transfer
  WHERE transfer_id = $1
`

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

const lockAccount = `
UPDATE account
  SET pending_transfer = CASE WHEN (pending_transfer IS NULL) THEN ($1)
	ELSE (pending_transfer)
  END, pending_amount = CASE WHEN (pending_amount = 0) THEN $2
	ELSE (pending_amount)
  END
  WHERE bic = $3 AND ban = $4 AND balance IS NOT NULL
  RETURNING *
`

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

const unlockAccount = `
UPDATE account
  SET pending_transfer = NULL, pending_amount = 0
  WHERE bic = $1 AND ban = $2
  AND balance IS NOT NULL AND pending_transfer = $3
`

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

const updateBalance = `
UPDATE account
  SET pending_amount = 0, balance = $1
  WHERE bic = $2 AND ban = $3
  AND balance IS NOT NULL AND pending_transfer = $4
`

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

const fetchBalance = `
SELECT balance, pending_amount
  FROM account
  WHERE bic = $1 AND ban = $2
`

func (self *PostgresCluster) FetchBalance(bic string, ban string) (*inf.Dec, *inf.Dec, error) {
	row := self.pool.QueryRow(context.Background(), fetchBalance, bic, ban)
	var balance, pendingAmount inf.Dec
	err := row.Scan(&balance, &pendingAmount)
	if err != nil {
		return nil, nil, err
	}
	return &balance, &pendingAmount, nil
}

const fetchDeadTransfers = `
SELECT transfer_id FROM transfer
`

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
		err := tx.Rollback(context.Background())
		if err != nil && err != pgx.ErrTxClosed {
			panic(merry.WithCause(ErrConsistencyViolation, fmt.Errorf("failed to rollback transaction: %s", err)))
		}
	}()

	_, err = tx.Exec(
		ctx,
		insertTransfer,
		transfer.Id,
		transfer.Acs[0].Bic,
		transfer.Acs[0].Ban,
		transfer.Acs[1].Bic,
		transfer.Acs[1].Ban,
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

	// update balance
	row := tx.QueryRow(
		ctx,
		`
		UPDATE account SET balance = balance - $1 WHERE bic = $2 and ban = $3 RETURNING balance;
		`,
		transfer.Amount.UnscaledBig().Int64(),
		transfer.Acs[0].Bic,
		transfer.Acs[0].Ban,
	)
	var balance int64
	if balance < 0 {
		return ErrInsufficientFunds
	}
	if err := row.Scan(&balance); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgerrcode.IsTransactionRollback(pgErr.Code) {
				return ErrTxRollback
			}
		}
		// failed to find first account
		if err == pgx.ErrNoRows {
			return ErrNoRows
		}
		return merry.Prepend(err, "failed to update first balance")
	}

	// update balance
	res, err := tx.Exec(
		ctx,
		`
		UPDATE account SET balance = balance + $1 WHERE bic = $2 and ban = $3;
		`,
		transfer.Amount.UnscaledBig().Int64(),
		transfer.Acs[1].Bic,
		transfer.Acs[1].Ban,
	)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgerrcode.IsTransactionRollback(pgErr.Code) {
				return ErrTxRollback
			}
		}
		return merry.Prepend(err, "failed to update second balance")
	}
	// failed to find second account
	if res.RowsAffected() != 1 {
		return ErrNoRows
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

	return nil
}