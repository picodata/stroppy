package cluster

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/ansel1/merry/v2"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	llog "github.com/sirupsen/logrus"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"
	environ "github.com/ydb-platform/ydb-go-sdk-auth-environ"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/table"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/options"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/result"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/result/named"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"
	"gitlab.com/picodata/stroppy/internal/model"
	"gopkg.in/inf.v0"
)

const (
	schemeErr    string = "Path does not exist"
	stroppyDir   string = "stroppy"
	stroppyAgent string = "stroppy 1.0"
	// default operation timeout.
	defaultTimeout = time.Second * 10
	// partitioning settings for accounts and transfers tables.
	partitionsMinCount  = 100
	partitionsMaxMbytes = 12
	poolSizeOverhead    = 10
)

var errIllegalNilOutput = errors.New(
	"Illegal nil output value of balance column for srcdst account statement",
)

type YandexDBCluster struct {
	ydbConnection       ydb.Connection
	yqlInsertAccount    string
	yqlUpsertTransfer   string
	yqlSelectSrcDstAcc  string
	yqlUpsertSrcDstAcc  string
	yqlSelectBalanceAcc string
}

func envExists(key string) bool {
	if value, ok := os.LookupEnv(key); ok {
		return len(value) > 0
	}

	return false
}

func envConfigured() bool {
	return (envExists("YDB_SERVICE_ACCOUNT_KEY_FILE_CREDENTIALS") ||
		envExists("YDB_METADATA_CREDENTIALS") ||
		envExists("YDB_ACCESS_TOKEN_CREDENTIALS"))
}

func NewYandexDBCluster(
	ydbContext context.Context,
	dbURL string,
	poolSize uint64,
) (*YandexDBCluster, error) {
	llog.Infof("Establishing connection to YDB on %s with poolSize %d", dbURL, poolSize)

	var (
		database ydb.Connection
		err      error
	)

	if envConfigured() {
		llog.Infoln("NOTE: YDB connection credentials are configured through the environment")

		database, err = ydb.Open(ydbContext, dbURL,
			ydb.WithUserAgent(stroppyAgent),
			ydb.WithSessionPoolSizeLimit(int(poolSize+poolSizeOverhead)),
			ydb.WithSessionPoolIdleThreshold(defaultTimeout),
			ydb.WithDiscoveryInterval(defaultTimeout),
			environ.WithEnvironCredentials(ydbContext),
		)
	} else {
		database, err = ydb.Open(ydbContext, dbURL,
			ydb.WithUserAgent(stroppyAgent),
			ydb.WithSessionPoolSizeLimit(int(poolSize+poolSizeOverhead)),
			ydb.WithSessionPoolIdleThreshold(defaultTimeout),
			ydb.WithDiscoveryInterval(defaultTimeout),
		)
	}

	if err != nil {
		return nil, errors.Wrap(err, "Error creating YDB connection holder")
	}

	return &YandexDBCluster{
		ydbConnection:       database,
		yqlUpsertTransfer:   expandYql(yqlUpsertTransfer),
		yqlSelectSrcDstAcc:  expandYql(yqlSelectSrcDstAccount),
		yqlUpsertSrcDstAcc:  expandYql(yqlUpsertSrcDstAccount),
		yqlInsertAccount:    expandYql(yqlInsertAccount),
		yqlSelectBalanceAcc: expandYql(yqlSelectBalanceAccount),
	}, nil
}

func (*YandexDBCluster) GetClusterType() DBClusterType {
	return YandexDBClusterType
}

func (ydbCluster *YandexDBCluster) FetchSettings() (Settings, error) {
	var (
		err            error
		clusterSettins Settings
	)

	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	tableFullPath := path.Join(ydbCluster.ydbConnection.Name(), stroppyDir, "settings")

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			var queryResult result.StreamResult
			if queryResult, err = ydbSession.StreamReadTable(
				ydbContext,
				tableFullPath,
			); err != nil {
				return errors.Wrap(err, "failed to reading table in stream")
			}

			llog.Traceln("Settings successfully fetched from ydb")

			defer func() {
				_ = queryResult.Close()
			}()

			var (
				key   string
				value string
			)

			for queryResult.NextResultSet(ydbContext) {
				for queryResult.NextRow() {
					if err = queryResult.ScanNamed(
						named.OptionalWithDefault("key", &key),
						named.OptionalWithDefault("value", &value),
					); err != nil {
						return errors.Wrap(err, "failed ot scan parameters")
					}
					switch key {
					case "count":
						if clusterSettins.Count, err = strconv.Atoi(value); err != nil {
							return errors.Wrap(err, "failed to convert count into integer")
						}
					case "seed":
						if clusterSettins.Seed, err = strconv.Atoi(value); err != nil {
							return errors.Wrap(err, "failed to convert seed into integer")
						}
					}
					llog.Tracef(
						"Settings{ key: %s, value: %s }",
						key,
						value,
					)
				}
			}

			if err = queryResult.Err(); err != nil {
				return errors.Wrap(err, "failed retrieve query result")
			}

			return nil
		},
	); err != nil {
		return clusterSettins, errors.Wrap(err, "Error fetching data from settings table")
	}

	return clusterSettins, nil
}

func (ydbCluster *YandexDBCluster) MakeAtomicTransfer(
	transfer *model.Transfer, //nolint
	clientID uuid.UUID,
) error {
	var err error

	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	amount := transfer.Amount.UnscaledBig().Int64()

	if err = ydbCluster.ydbConnection.Table().DoTx(
		ydbContext,
		func(ctx context.Context, tx table.TransactionActor) error {
			// Select from account table
			var query result.Result
			query, err = tx.Execute(
				ctx, ydbCluster.yqlSelectSrcDstAcc,
				table.NewQueryParameters(
					table.ValueParam("src_bic",
						types.BytesValueFromString(transfer.Acs[0].Bic)),
					table.ValueParam("src_ban",
						types.BytesValueFromString(transfer.Acs[0].Ban)),
					table.ValueParam("dst_bic",
						types.BytesValueFromString(transfer.Acs[1].Bic)),
					table.ValueParam("dst_ban",
						types.BytesValueFromString(transfer.Acs[1].Ban)),
				),
				options.WithKeepInCache(true),
			)
			if err != nil {
				return errors.Wrap(err, "failed to execute transaction")
			}
			defer func() {
				_ = query.Close()
			}()

			for query.NextResultSet(ctx) {
				// Expect to have 2 rows - source and destination accounts.
				// In case of 0 or 1 rows something is missing.
				if query.CurrentResultSet().RowCount() != 2 { //nolint:gomnd // not magic number
					llog.Tracef(
						"missing transfer: src_bic: %s, src_ban: %s dst_bic: %s, dst_ban: %s",
						transfer.Acs[0].Bic, transfer.Acs[0].Ban,
						transfer.Acs[1].Bic, transfer.Acs[1].Ban,
					)

					return ErrNoRows
				}
				for query.NextRow() {
					var srcdst int32
					var balance *int64
					if err = query.Scan(&srcdst, &balance); err != nil {
						return errors.Wrap(err, "failed to scan account balance")
					}
					if balance == nil {
						return errIllegalNilOutput
					}
					switch srcdst {
					case 1: // need to check the source account balance
						if *balance < amount {
							return ErrInsufficientFunds
						}
					case 2: //nolint:gomnd // nothing to do on the destination account
					default: // something strange to be reported
						return merry.Errorf(
							"Illegal srcdst value %d for srcdst account statement",
							srcdst,
						)
					}
				}
			}
			if err = query.Err(); err != nil {
				return errors.Wrap(err, "failed to retrieve query status")
			}

			// Upsert the new row to the transfer table
			_, err = tx.Execute(
				ctx, ydbCluster.yqlUpsertTransfer,
				table.NewQueryParameters(
					table.ValueParam("transfer_id",
						types.BytesValueFromString(transfer.Id.String())),
					table.ValueParam("src_bic",
						types.BytesValueFromString(transfer.Acs[0].Bic)),
					table.ValueParam("src_ban",
						types.BytesValueFromString(transfer.Acs[0].Ban)),
					table.ValueParam("dst_bic",
						types.BytesValueFromString(transfer.Acs[1].Bic)),
					table.ValueParam("dst_ban",
						types.BytesValueFromString(transfer.Acs[1].Ban)),
					table.ValueParam("amount",
						types.Int64Value(amount)),
					table.ValueParam("state",
						types.BytesValueFromString("complete")),
				),
				options.WithKeepInCache(true),
			)
			if err != nil {
				return errors.Wrap(err, "failed to execute transaction")
			}

			// Update two balances in the account table.
			_, err = tx.Execute(
				ctx, ydbCluster.yqlUpsertSrcDstAcc,
				table.NewQueryParameters(
					table.ValueParam("src_bic",
						types.BytesValueFromString(transfer.Acs[0].Bic)),
					table.ValueParam("src_ban",
						types.BytesValueFromString(transfer.Acs[0].Ban)),
					table.ValueParam("dst_bic",
						types.BytesValueFromString(transfer.Acs[1].Bic)),
					table.ValueParam("dst_ban",
						types.BytesValueFromString(transfer.Acs[1].Ban)),
					table.ValueParam("amount",
						types.Int64Value(transfer.Amount.UnscaledBig().Int64())),
				),
				options.WithKeepInCache(true),
			)
			if err != nil {
				return errors.Wrap(err, "failed to execute transaction")
			}

			return nil
		},
		// Mark the transaction idempotent to allow retries.
		table.WithIdempotent(),
	); err != nil {
		return errors.Wrap(err, "failed to execute 'Do' procedure")
	}

	return nil
}

func (ydbCluster *YandexDBCluster) FetchAccounts() ([]model.Account, error) {
	var err error

	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	tablePath := path.Join(stroppyDir, "account")
	selectStmnt := fmt.Sprintf("SELECT bic, ban, balance FROM `%s`", tablePath)

	var accs []model.Account

	if err = ydbCluster.ydbConnection.Table().Do(

		ydbContext,
		func(ctx context.Context, sess table.Session) error {
			var rows result.StreamResult

			rows, err = sess.StreamExecuteScanQuery(ctx, selectStmnt, nil)
			if err != nil {
				return errors.Wrap(err, "failed to execute scan query")
			}
			defer func() {
				_ = rows.Close()
			}()
			for rows.NextResultSet(ydbContext) {
				for rows.NextRow() {
					var Balance int64
					var acc model.Account
					if err = rows.Scan(&acc.Bic, &acc.Ban, &Balance); err != nil {
						return errors.Wrap(err, "failed to scan columns values")
					}
					dec := new(inf.Dec)
					dec.SetUnscaled(Balance)
					acc.Balance = dec
					accs = append(accs, acc)
				}
			}

			return nil
		},
	); err != nil {
		return nil, errors.Wrap(err, "failed to fetch accounts")
	}

	return accs, nil
}

func (ydbCluster *YandexDBCluster) FetchBalance(
	bic string,
	ban string,
) (*inf.Dec, *inf.Dec, error) {
	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	var (
		err  error
		rows result.Result
	)

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, rows, err = ydbSession.Execute(
				ydbContext, table.OnlineReadOnlyTxControl(),
				ydbCluster.yqlSelectBalanceAcc,
				table.NewQueryParameters(
					table.ValueParam("bic", types.BytesValueFromString(bic)),
					table.ValueParam("ban", types.BytesValueFromString(ban)),
				),
				options.WithKeepInCache(true),
			); err != nil {
				return errors.Wrap(err, "failed to execute query")
			}

			return nil
		},
	); err != nil {
		return nil, nil, errors.Wrap(err, "failed to execute 'Do' procedure")
	}

	defer func() {
		_ = rows.Close()
	}()

	var balance, pendingAmount inf.Dec

	if rows.NextResultSet(ydbContext) {
		if rows.NextRow() {
			err = rows.Scan(&balance, &pendingAmount)
			if err != nil {
				return nil, nil, errors.Wrap(err, "failed to scan columns values")
			}

			return &balance, &pendingAmount, nil
		}
	}

	return nil, nil, errors.Errorf("No amount for bic %s and ban %s", bic, ban)
}

func (ydbCluster *YandexDBCluster) FetchTotal() (*inf.Dec, error) {
	var (
		err         error
		queryResult result.Result
		amount      int64
	)

	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, queryResult, err = ydbSession.Execute(
				ydbContext, table.OnlineReadOnlyTxControl(),
				fmt.Sprintf("SELECT amount FROM `%s/checksum` WHERE name = %q;", stroppyDir, "total"),
				nil,
				options.WithKeepInCache(true),
			); err != nil {
				return errors.Wrap(err, "failed to execute 'Do' procedure")
			}
			return nil
		},
	); err != nil {
		return nil, errors.Wrap(err, "failed to select totals from checksum table")
	}
	defer func() {
		_ = queryResult.Close()
	}()

	for queryResult.NextResultSet(ydbContext) {
		for queryResult.NextRow() {
			if err = queryResult.ScanNamed(
				named.Required("amount", &amount),
			); err != nil {
				return nil, errors.Wrap(err, "failed to scan columns values")
			}
		}
	}

	llog.Tracef("Checksum row with name 'total' has amount %d", amount)

	if amount == 0 {
		return nil, ErrNoRows
	}

	return inf.NewDec(amount, 0), nil
}

func (ydbCluster *YandexDBCluster) CheckBalance() (*inf.Dec, error) {
	var (
		err          error
		queryResult  result.Result
		totalBalance int64
	)

	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, queryResult, err = ydbSession.Execute(
				ydbContext, table.OnlineReadOnlyTxControl(),
				fmt.Sprintf("SELECT SUM(balance) AS total FROM `%s/account`;", stroppyDir),
				nil,
				options.WithKeepInCache(true),
			); err != nil {
				return errors.Wrap(err, "failed to execute 'Do' procedure")
			}

			return nil
		},
	); err != nil {
		return nil, errors.Wrap(err, "failed to compute the total balance on the account table")
	}
	defer func() {
		_ = queryResult.Close()
	}()

	totalBalance = 0
	for queryResult.NextResultSet(ydbContext) {
		for queryResult.NextRow() {
			if err = queryResult.ScanNamed(
				named.OptionalWithDefault("total", &totalBalance),
			); err != nil {
				return nil, errors.Wrap(err, "failed to scan columns values")
			}

			llog.Tracef("Account{ totalBalance: %d }", totalBalance)
		}
	}

	return inf.NewDec(totalBalance, 0), nil
}

func (ydbCluster *YandexDBCluster) PersistTotal(total inf.Dec) error {
	var err error

	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, _, err = ydbSession.Execute(
				ydbContext, table.DefaultTxControl(),
				fmt.Sprintf("DECLARE $name AS String;"+
					"DECLARE $amount AS Int64;"+
					"UPSERT INTO `%s/checksum` (name, amount) "+
					"VALUES ($name, $amount)",
					stroppyDir,
				),
				table.NewQueryParameters(
					table.ValueParam("name", types.BytesValueFromString("total")),
					table.ValueParam("amount", types.Int64Value(total.UnscaledBig().Int64())),
				),
				options.WithKeepInCache(true),
			); err != nil {
				return errors.Wrap(err, "failed to scan columns values")
			}
			return nil
		},
		table.WithIdempotent(),
	); err != nil {
		return errors.Wrap(err, "failed to insert data into checksum table")
	}

	return nil
}

func (ydbCluster *YandexDBCluster) BootstrapDB(count uint64, seed int) error {
	var err error

	llog.Infof("Creating the folders and tables...")

	ydbContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	prefix := path.Join(ydbCluster.ydbConnection.Name(), stroppyDir)

	if err = createStroppyDirectory(
		ydbContext,
		ydbCluster.ydbConnection,
		prefix,
	); err != nil {
		return err
	}

	if err = createSettingsTable(
		ydbContext,
		ydbCluster.ydbConnection.Table(),
		prefix,
	); err != nil {
		return err
	}

	if err = createAccountTable(
		ydbContext,
		ydbCluster.ydbConnection.Table(),
		prefix,
	); err != nil {
		return err
	}

	if err = createTransferTable(
		ydbContext,
		ydbCluster.ydbConnection.Table(),
		prefix,
	); err != nil {
		return err
	}

	if err = createChecksumTable(
		ydbContext,
		ydbCluster.ydbConnection.Table(),
		prefix,
	); err != nil {
		return err
	}

	if err = upsertSettings(
		ydbContext,
		ydbCluster.ydbConnection.Table(),
		fmt.Sprintf("%d", count),
		fmt.Sprintf("%d", seed),
	); err != nil {
		return err
	}

	return nil
}

func createSettingsTable( //nolint:dupl // because it golang
	ydbContext context.Context,
	ydbClient table.Client, prefix string,
) error {
	var err error

	tabname := path.Join(prefix, "settings")
	if err = recreateTable(
		ydbContext, ydbClient, tabname,
		func(ctx context.Context, session table.Session) error {
			if err = session.CreateTable(
				ctx, tabname,
				options.WithColumn("key", types.Optional(types.TypeString)),
				options.WithColumn("value", types.Optional(types.TypeString)),
				options.WithPrimaryKeyColumn("key"),
			); err != nil {
				return errors.Wrap(err, "failed to create table")
			}

			return nil
		},
	); err != nil {
		return errors.Wrap(err, "failed to recreate settings table")
	}

	return nil
}

func createAccountTable(
	ydbContext context.Context,
	ydbClient table.Client, prefix string,
) error {
	var err error

	tabname := path.Join(prefix, "account")
	if err = recreateTable(
		ydbContext, ydbClient, tabname,
		func(ctx context.Context, session table.Session) error {
			if err = session.CreateTable(
				ctx, tabname,
				options.WithColumn("bic", types.Optional(types.TypeString)),
				options.WithColumn("ban", types.Optional(types.TypeString)),
				options.WithColumn("balance", types.Optional(types.TypeInt64)),
				options.WithPrimaryKeyColumn("bic", "ban"),
				options.WithPartitioningSettings(
					options.WithPartitioningByLoad(options.FeatureEnabled),
					options.WithPartitioningBySize(options.FeatureEnabled),
					options.WithMinPartitionsCount(partitionsMinCount),
					options.WithPartitionSizeMb(partitionsMaxMbytes),
				),
			); err != nil {
				return errors.Wrap(err, "failed to create table")
			}

			return nil
		},
	); err != nil {
		return errors.Wrap(err, "failed to recreate account table")
	}

	return nil
}

func createTransferTable(
	ydbContext context.Context,
	ydbClient table.Client, prefix string,
) error {
	var err error

	tabname := path.Join(prefix, "transfer")
	if err = recreateTable(
		ydbContext, ydbClient, tabname,
		func(ctx context.Context, session table.Session) error {
			if err = session.CreateTable(
				ctx, tabname,
				options.WithColumn("transfer_id", types.Optional(types.TypeString)),
				options.WithColumn("src_bic", types.Optional(types.TypeString)),
				options.WithColumn("src_ban", types.Optional(types.TypeString)),
				options.WithColumn("dst_bic", types.Optional(types.TypeString)),
				options.WithColumn("dst_ban", types.Optional(types.TypeString)),
				options.WithColumn("amount", types.Optional(types.TypeInt64)),
				options.WithColumn("state", types.Optional(types.TypeString)),
				options.WithColumn("client_id", types.Optional(types.TypeString)),
				options.WithColumn("client_timestamp", types.Optional(types.TypeTimestamp)),
				options.WithPrimaryKeyColumn("transfer_id"),
				options.WithPartitioningSettings(
					options.WithPartitioningByLoad(options.FeatureEnabled),
					options.WithPartitioningBySize(options.FeatureEnabled),
					options.WithMinPartitionsCount(partitionsMinCount),
					options.WithPartitionSizeMb(partitionsMaxMbytes),
				),
			); err != nil {
				return errors.Wrap(err, "failed to create table")
			}

			return nil
		},
	); err != nil {
		return errors.Wrap(err, "failed to recreate account table")
	}

	return nil
}

func createChecksumTable( //nolint:dupl // because it golang
	ydbContext context.Context,
	ydbClient table.Client, prefix string,
) error {
	var err error

	tabname := path.Join(prefix, "checksum")
	if err = recreateTable(
		ydbContext, ydbClient, tabname,
		func(ctx context.Context, session table.Session) error {
			if err = session.CreateTable(
				ctx, tabname,
				options.WithColumn("name", types.Optional(types.TypeString)),
				options.WithColumn("amount", types.Optional(types.TypeInt64)),
				options.WithPrimaryKeyColumn("name"),
			); err != nil {
				return errors.Wrap(err, "failed to create table")
			}

			return nil
		},
	); err != nil {
		return errors.Wrap(err, "failed to recreate checksum table")
	}

	return nil
}

func recreateTable(
	ydbContext context.Context,
	ydbClient table.Client,
	tablePath string,
	createFunc func(ctx context.Context, session table.Session) error,
) error {
	var err error

	if err = ydbClient.Do(
		ydbContext,
		func(ctx context.Context, session table.Session) error {
			if err = session.DropTable(ctx, tablePath); err != nil {
				if strings.Contains(err.Error(), schemeErr) {
					llog.Debugf(
						"Database table '%s' does not exists at this moment in YDB cluster",
						tablePath,
					)
				} else {
					return errors.Wrap(err, "failed to execute 'Do' procedure")
				}
			}

			return nil
		},
	); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Error droping table %s", tablePath))
	}

	if err = ydbClient.Do(ydbContext, createFunc); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Error creating table %s", tablePath))
	}

	llog.Infof("Table created: %s", tablePath)
	return nil
}

func createStroppyDirectory(
	ydbContext context.Context,
	ydbConnection ydb.Connection,
	ydbDirPath string,
) error {
	if err := ydbConnection.Scheme().RemoveDirectory(
		ydbContext,
		ydbDirPath,
	); err != nil {
		llog.Debugf("Database directory '%s' does not exists in YDB cluster", ydbDirPath)
	}

	if err := ydbConnection.Scheme().MakeDirectory(
		ydbContext,
		ydbDirPath,
	); err != nil {
		return errors.Wrap(
			err,
			fmt.Sprintf("Error creating directory %s", ydbDirPath),
		)
	}

	llog.Infof("Directory created: %s", ydbDirPath)

	return nil
}

func upsertSettings(
	ydbContext context.Context,
	ydbTableClient table.Client,
	count, seed string,
) (err error) {
	if err = ydbTableClient.Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, _, err = ydbSession.Execute(
				ydbContext, table.DefaultTxControl(),
				fmt.Sprintf("DECLARE $key AS List<String>;"+
					"DECLARE $value AS List<String>;"+
					"UPSERT INTO `%s/settings` (key, value) "+
					"VALUES ($key[0], $value[0]), ($key[1], $value[1])",
					stroppyDir,
				),
				table.NewQueryParameters(
					table.ValueParam("key", types.ListValue(
						types.BytesValueFromString("count"),
						types.BytesValueFromString("seed"),
					)),
					table.ValueParam("value", types.ListValue(
						types.BytesValueFromString(count),
						types.BytesValueFromString(seed),
					)),
				),
				options.WithKeepInCache(true),
			); err != nil {
				return errors.Wrap(err, "failed execute execute query")
			}

			return nil
		},
		table.WithIdempotent(),
	); err != nil {
		return errors.Wrap(err, "failed to execute 'Do' procedure")
	}

	llog.Infoln("Settings successfully inserted")

	return nil
}

func (ydbCluster *YandexDBCluster) InsertAccount(acc model.Account) error {
	var err error

	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, _, err = ydbSession.Execute(
				ydbContext, table.DefaultTxControl(),
				ydbCluster.yqlInsertAccount,
				table.NewQueryParameters(
					table.ValueParam("bic", types.BytesValueFromString(acc.Bic)),
					table.ValueParam("ban", types.BytesValueFromString(acc.Ban)),
					table.ValueParam("balance", types.Int64Value(acc.Balance.UnscaledBig().Int64())),
				),
				options.WithKeepInCache(true),
			); err != nil {
				return errors.Wrap(err, "failed to execute 'Do' procedure")
			}
			return nil
		},
		table.WithIdempotent(),
	); err != nil {
		if ydb.IsOperationError(err, Ydb.StatusIds_PRECONDITION_FAILED) { //nolint
			return ErrDuplicateKey
		}

		return errors.Wrap(err, "Error inserting data into account table")
	}

	return nil
}

func (ydbCluster *YandexDBCluster) InsertTransfer(transfer *model.Transfer) error {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) DeleteTransfer(
	transferID model.TransferId,
	clientID uuid.UUID,
) error {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) SetTransferClient(
	clientID uuid.UUID,
	transferID model.TransferId,
) error {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) FetchTransferClient(
	transferID model.TransferId,
) (*uuid.UUID, error) {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) ClearTransferClient(
	transferID model.TransferId,
	clientID uuid.UUID,
) error {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) SetTransferState(
	state string,
	transferID model.TransferId,
	clientID uuid.UUID,
) error {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) FetchTransfer(
	transferID model.TransferId,
) (*model.Transfer, error) {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) FetchDeadTransfers() ([]model.TransferId, error) {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) UpdateBalance(
	balance *inf.Dec,
	bic string,
	ban string,
	transferID model.TransferId,
) error {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) LockAccount(
	transferID model.TransferId,
	pendingAmount *inf.Dec,
	bic string,
	ban string,
) (*model.Account, error) {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) UnlockAccount(
	bic string,
	ban string,
	transferID model.TransferId,
) error {
	panic("unimplemented!")
}

// TODO: check possibility of collecting statistics for YDB.
func (ydbCluster *YandexDBCluster) StartStatisticsCollect(_ time.Duration) error {
	llog.Debugln("statistic for YDB not implemeted yet, watch grafana metrics, please")

	return nil
}

// Substitute directory path into the YQL template,
// replacing the double quote characters with backticks.
func expandYql(query string) string {
	retval := strings.ReplaceAll(query, "&{stroppyDir}", stroppyDir)
	retval = strings.ReplaceAll(retval, `"`, "`")

	return retval
}
