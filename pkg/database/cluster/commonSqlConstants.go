package cluster

const (
	bootstrapScript = `
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

CREATE TABLE IF NOT EXISTS history (
	id            UUID PRIMARY KEY, -- history item UUID
	transfer_id    UUID, -- id of corresponding transfer
	account_bic    TEXT, -- source bank identification code
	account_ban    TEXT, -- source bank account number
	old_balance    DECIMAL, -- old balance value
	new_balance    DECIMAL, -- new balance value
	operation_time TIMESTAMP -- time, when balance change operation was performed
);
TRUNCATE history;

CREATE TABLE IF NOT EXISTS checksum (
	name TEXT PRIMARY KEY,
	amount DECIMAL
);
TRUNCATE checksum;
`

	// --- fetching ------------------
	fetchTotal = `SELECT amount FROM checksum WHERE name = 'total;'`

	fetchAccounts = `SELECT * FROM account`

	fetchSettings = `SELECT value FROM setting WHERE KEY in ('count', 'seed');`

	checkBalance = `SELECT SUM(balance) FROM account`

	// --- insertions ----------------
	upsertAccount = `INSERT INTO account (bic, ban, balance, pending_amount) VALUES ($1, $2, $3, 0);`

	insertSetting = `INSERT INTO setting (key, value) VALUES ($1, $2);`

	persistTotal = `INSERT INTO checksum (name, amount) VALUES('total', $1)
ON CONFLICT (name) DO UPDATE SET amount = excluded.amount;`
)

const timeOutSettings = 5
