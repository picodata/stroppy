package cluster

import "time"

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

// --- fetching ------------------
const (
	fetchTotal = `SELECT amount FROM checksum WHERE name = 'total;'`

	fetchAccounts = `SELECT * FROM account`

	fetchSettings = `SELECT value FROM setting WHERE KEY in ('count', 'seed');`

	checkBalance = `SELECT SUM(balance) FROM account`

	fetchTransfer = `SELECT src_bic, src_ban, dst_bic, dst_ban, amount, state
  FROM transfer WHERE transfer_id = $1;`

	fetchTransferClient = `SELECT client_id
  FROM transfer WHERE transfer_id = $1;`

	fetchBalance = `SELECT balance
  FROM account WHERE bic = $1 AND ban = $2;`

	fetchDeadTransfers = `SELECT transfer_id FROM transfer;`
)

// --- insertions ----------------
const (
	upsertAccount = `INSERT INTO account (bic, ban, balance) VALUES ($1, $2, $3);`

	insertSetting = `INSERT INTO setting (key, value) VALUES ($1, $2);`

	persistTotal = `INSERT INTO checksum (name, amount) VALUES('total', $1)
	ON CONFLICT (name) DO UPDATE SET amount = excluded.amount;`

	// Client id has to be updated separately to let it expire
	insertTransfer = `INSERT INTO transfer (transfer_id, src_bic, src_ban, dst_bic, dst_ban, amount, state)
	VALUES ($1, $2, $3, $4, $5, $6, 'complete');`
)

// --- data update ----------------
const (
	setTransferState = `UPDATE transfer SET state = $1 WHERE transfer_id = $2
	AND amount IS NOT NULL AND client_id = $3 AND client_timestamp > now() - interval'30 second';`

	setTransferClient = `UPDATE transfer SET client_id = $1, client_timestamp = now()
	WHERE transfer_id = $2 AND amount IS NOT NULL;`

	clearTransferClient = `UPDATE transfer SET client_id = NULL
	WHERE transfer_id = $1 AND amount IS NOT NULL AND client_id = $2;`

	deleteTransfer = `DELETE FROM transfer WHERE transfer_id = $1
	AND client_id = $2 AND client_timestamp > now() - interval '30 second';`
	/*
	   	lockAccount = `UPDATE account
	     SET pending_transfer = CASE WHEN (pending_transfer IS NULL) THEN ($1)
	   	ELSE (pending_transfer)
	     END, pending_amount = CASE WHEN (pending_amount = 0) THEN $2
	   	ELSE (pending_amount)
	     END
	     WHERE bic = $3 AND ban = $4 AND balance IS NOT NULL
	     RETURNING *
	   `

	   	unlockAccount = `
	   UPDATE account
	     SET pending_transfer = NULL, pending_amount = 0
	     WHERE bic = $1 AND ban = $2
	     AND balance IS NOT NULL AND pending_transfer = $3
	   `

	   	updateBalance = `
	   UPDATE account
	     SET pending_amount = 0, balance = $1
	     WHERE bic = $2 AND ban = $3
	     AND balance IS NOT NULL AND pending_transfer = $4
	   `*/
)

const (
	timeOutSettings = 5
	txTimeout       = 5 * time.Second
)
