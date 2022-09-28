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
	llog "github.com/sirupsen/logrus"
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
	schemeErr      string = "Path does not exist"
	stroppyDir     string = "stroppy"
	stroppyAgent   string = "stroppy 1.0"
	defaultTimeout        = time.Second * 5
)

type YandexDBCluster struct {
	ydbConnection ydb.Connection
	at_transfer   string
	at_select     string
	at_unified    string
	acc_insert    string
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

func NewYandexDBCluster(ydbContext context.Context, dbURL string, poolSize int) (*YandexDBCluster, error) {
	llog.Infof("Establishing connection to YDB on %s with poolSize %d", dbURL, poolSize)

	var (
		database ydb.Connection
		err      error
	)

	if envConfigured() {
		database, err = ydb.Open(ydbContext, dbURL,
			ydb.WithUserAgent(stroppyAgent),
			ydb.WithSessionPoolSizeLimit(poolSize+10),
			ydb.WithSessionPoolIdleThreshold(defaultTimeout),
			ydb.WithDiscoveryInterval(defaultTimeout),
			environ.WithEnvironCredentials(ydbContext),
		)
	} else {
		database, err = ydb.Open(ydbContext, dbURL,
			ydb.WithUserAgent(stroppyAgent),
			ydb.WithSessionPoolSizeLimit(poolSize+10),
			ydb.WithSessionPoolIdleThreshold(defaultTimeout),
			ydb.WithDiscoveryInterval(defaultTimeout),
		)
	}

	if err != nil {
		return nil, merry.Prepend(err, "Error then creating YDB connection holder")
	}

	// Build SQL text just once
	tablePath := path.Join(stroppyDir, "transfer")
	transferStmnt := insertEscapedPath(insertYdbTransfer, tablePath)
	tablePath = path.Join(stroppyDir, "account")
	selectStmnt := insertEscapedPath(srcAndDstYdbSelect, tablePath, tablePath)
	unifiedStmnt := insertEscapedPath(unifiedTransfer, tablePath, tablePath, tablePath)
	insertAccStmnt := fmt.Sprintf("DECLARE $bic AS String; "+
		"DECLARE $ban AS String; DECLARE $balance AS Int64; "+
		"UPSERT INTO `%s` (bic, ban, balance) VALUES ($bic, $ban, $balance)",
		tablePath,
	)

	return &YandexDBCluster{
		ydbConnection: database,
		at_transfer:   transferStmnt,
		at_select:     selectStmnt,
		at_unified:    unifiedStmnt,
		acc_insert:    insertAccStmnt,
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

	tableFullPath := path.Join(ydbCluster.ydbConnection.Name(), stroppyDir, "settings")
	ydbContext, ctxCloseFn := context.WithCancel(context.Background())

	defer ctxCloseFn()

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			var queryResult result.StreamResult
			if queryResult, err = ydbSession.StreamReadTable(
				ydbContext,
				tableFullPath,
			); err != nil {
				return merry.Prepend(err, "failed to fetch rows")
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
						return merry.Prepend(err, "failed to map fetch result to values")
					}
					switch key {
					case "count":
						if clusterSettins.Count, err = strconv.Atoi(value); err != nil {
							return merry.Prepend(err, "failed to convert count into integer")
						}
					case "seed":
						if clusterSettins.Seed, err = strconv.Atoi(value); err != nil {
							return merry.Prepend(err, "failed to convert seed into integer")
						}
					}
					llog.Tracef(
						"Settings{ key: %s, value: %s }",
						key,
						value,
					)
				}
			}

			if queryResult.Err() != nil {
				return merry.Prepend(queryResult.Err(), "failed to work with response result")
			}

			return nil
		},
	); err != nil {
		return clusterSettins, merry.Prepend(err, "Error then fetching setting from settings table")
	}

	return clusterSettins, nil
}

func (ydbCluster *YandexDBCluster) MakeAtomicTransfer(
	transfer *model.Transfer, //nolint
	clientID uuid.UUID,
) error {
	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	txc := table.TxControl(
		table.BeginTx(table.WithSerializableReadWrite()),
		table.CommitTx(),
	)

	return ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ctx context.Context, sess table.Session) (err error) {
			// Select from account table
			var qr result.Result
			_, qr, err = sess.Execute(
				ctx, txc, ydbCluster.at_select,
				table.NewQueryParameters(table.ValueParam(
					"params", types.StructValue(
						types.StructFieldValue(
							"src_bic",
							types.StringValue([]byte(transfer.Acs[0].Bic)),
						),
						types.StructFieldValue(
							"src_ban",
							types.StringValue([]byte(transfer.Acs[0].Ban)),
						),
						types.StructFieldValue(
							"dst_bic",
							types.StringValue([]byte(transfer.Acs[1].Bic)),
						),
						types.StructFieldValue(
							"dst_ban",
							types.StringValue([]byte(transfer.Acs[1].Ban)),
						),
					),
				)),
				options.WithKeepInCache(true),
			)
			if err != nil {
				return merry.Prepend(err, "failed to select from accounts table")
			}
			defer func() {
				_ = qr.Close()
			}()

			for qr.NextResultSet(ydbContext) {
				if qr.CurrentResultSet().RowCount() != 2 { //nolint // 2 is dst ans src rows count
					llog.Tracef(
						"missing transfer: src_bic: %s, src_ban: %s dst_bic: %s, dst_ban: %s",
						transfer.Acs[0].Bic, transfer.Acs[0].Ban,
						transfer.Acs[1].Bic, transfer.Acs[1].Ban,
					)
					return ErrNoRows
				}
			}
			if qr.Err() != nil {
				return merry.Prepend(qr.Err(), "failed to work with select result")
			}

			// Insert the new row to the transfer table
			_, _, err = sess.Execute(
				ctx, txc, ydbCluster.at_transfer,
				table.NewQueryParameters(table.ValueParam(
					"params", types.StructValue(
						types.StructFieldValue(
							"transfer_id",
							types.StringValue([]byte(transfer.Id.String())),
						),
						types.StructFieldValue(
							"src_bic",
							types.StringValue([]byte(transfer.Acs[0].Bic)),
						),
						types.StructFieldValue(
							"src_ban",
							types.StringValue([]byte(transfer.Acs[0].Ban)),
						),
						types.StructFieldValue(
							"dst_bic",
							types.StringValue([]byte(transfer.Acs[1].Bic)),
						),
						types.StructFieldValue(
							"dst_ban",
							types.StringValue([]byte(transfer.Acs[1].Ban)),
						),
						types.StructFieldValue(
							"amount",
							types.Int64Value(transfer.Amount.UnscaledBig().Int64()),
						),
						types.StructFieldValue(
							"state",
							types.StringValue([]byte("complete")),
						),
					))),
				options.WithKeepInCache(true),
			)
			if err != nil {
				return merry.Prepend(err, "failed to insert into transfer table")
			}

			// Update two balances in the account table
			_, _, err = sess.Execute(
				ctx, txc, ydbCluster.at_unified,
				table.NewQueryParameters(
					table.ValueParam("params", types.StructValue(
						types.StructFieldValue("src_bic", types.StringValue([]byte(transfer.Acs[0].Bic))),
						types.StructFieldValue("src_ban", types.StringValue([]byte(transfer.Acs[0].Ban))),
						types.StructFieldValue("dst_bic", types.StringValue([]byte(transfer.Acs[1].Bic))),
						types.StructFieldValue("dst_ban", types.StringValue([]byte(transfer.Acs[1].Ban))),
						types.StructFieldValue(
							"amount",
							types.Int64Value(transfer.Amount.UnscaledBig().Int64())),
					))),
				options.WithKeepInCache(true),
			)
			if err != nil {
				return merry.Prepend(err, "failed to execute unified transfer")
			}

			return nil
		},
		table.WithIdempotent(),
	)
}

func (ydbCluster *YandexDBCluster) FetchAccounts() ([]model.Account, error) {
	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	tablePath := path.Join(stroppyDir, "account")
	selectStmnt := fmt.Sprintf("SELECT bic, ban, balance FROM `%s`", tablePath)

	var accs []model.Account

	err := ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ctx context.Context, sess table.Session) (err error) {
			rows, err := sess.StreamExecuteScanQuery(ctx, selectStmnt, nil)
			if err != nil {
				return err
			}
			defer func() {
				_ = rows.Close()
			}()
			for rows.NextResultSet(ydbContext) {
				for rows.NextRow() {
					var Balance int64
					var acc model.Account
					if err := rows.Scan(&acc.Bic, &acc.Ban, &Balance); err != nil {
						return merry.Prepend(err, "failed to scan account for FetchAccounts")
					}
					dec := new(inf.Dec)
					dec.SetUnscaled(Balance)
					acc.Balance = dec
					accs = append(accs, acc)
				}
			}
			return nil
		},
	)
	if err != nil {
		return nil, merry.Prepend(err, "failed to fetch accounts")
	}

	return accs, nil
}

func (ydbCluster *YandexDBCluster) FetchBalance(
	bic string,
	ban string,
) (*inf.Dec, *inf.Dec, error) {
	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	tablePath := path.Join(stroppyDir, "account")
	selectStmnt := fmt.Sprintf("DECLARE $bic AS String; DECLARE $ban AS String; "+
		"SELECT balance, CAST(0 AS Int64) FROM `%s` WHERE bic = $bic AND ban = $ban", tablePath)

	readTx := table.TxControl(
		table.BeginTx(table.WithOnlineReadOnly()),
		table.CommitTx(),
	)

	var (
		err           error
		rows          result.Result
		balance       inf.Dec
		pendingAmount inf.Dec
	)

	found := false
	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, rows, err = ydbSession.Execute(
				ydbContext, readTx, selectStmnt,
				table.NewQueryParameters(
					table.ValueParam("bic", types.BytesValueFromString(bic)),
					table.ValueParam("ban", types.BytesValueFromString(ban)),
				),
				options.WithKeepInCache(true),
			); err != nil {
				return err
			}
			defer func() {
				_ = rows.Close()
			}()

			if rows.NextResultSet(ydbContext) {
				if rows.NextRow() {
					err = rows.Scan(&balance, &pendingAmount)
					if err != nil {
						return err
					}
					found = true
				}
			}
			return nil
		},
	); err != nil {
		return nil, nil, err
	}

	if !found {
		return nil, nil, merry.Errorf("No amount for bic %s and ban %s", bic, ban)
	}
	return &balance, &pendingAmount, nil
}

func (ydbCluster *YandexDBCluster) FetchTotal() (*inf.Dec, error) {
	var (
		err         error
		queryResult result.Result
		amount      int64
	)

	tablePath := path.Join(stroppyDir, "checksum")
	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	readTx := table.TxControl(
		table.BeginTx(table.WithOnlineReadOnly()),
		table.CommitTx(),
	)

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, queryResult, err = ydbSession.Execute(
				ydbContext,
				readTx,
				fmt.Sprintf("SELECT amount FROM `%s` WHERE name = %q;", tablePath, "total"),
				nil,
				options.WithKeepInCache(true),
			); err != nil {
				return merry.Prepend(err, "failed to execute select query")
			}
			return nil
		},
	); err != nil {
		return nil, merry.Prepend(err, "failed to do action with table")
	}

	defer func() {
		_ = queryResult.Close()
	}()

	for queryResult.NextResultSet(ydbContext) {
		for queryResult.NextRow() {
			if err = queryResult.ScanNamed(
				named.Required("amount", &amount),
			); err != nil {
				return nil, merry.Prepend(err, "failed to map fetch result to values")
			}

			llog.Tracef(
				"Checksum{ amount: %d }",
				amount,
			)
		}
	}

	llog.Tracef("Checksum row with name 'total' has amount %d", amount)

	if queryResult.Err() != nil {
		return nil, merry.Prepend(queryResult.Err(), "failed to work with response result")
	}

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

	tablePath := path.Join(stroppyDir, "account")
	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	readTx := table.TxControl(
		table.BeginTx(table.WithOnlineReadOnly()),
		table.CommitTx(),
	)

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, queryResult, err = ydbSession.Execute(
				ydbContext,
				readTx,
				fmt.Sprintf("SELECT SUM(balance) AS total FROM `%s`;", tablePath),
				nil,
				options.WithKeepInCache(true),
			); err != nil {
				return merry.Prepend(err, "failed to execute select query")
			}

			return nil
		},
	); err != nil {
		return nil, merry.Prepend(err, "failed to do action with table")
	}

	defer func() {
		_ = queryResult.Close()
	}()

	for queryResult.NextResultSet(ydbContext) {
		for queryResult.NextRow() {
			if err = queryResult.ScanNamed(
				named.OptionalWithDefault("total", &totalBalance),
			); err != nil {
				return nil, merry.Prepend(err, "failed to map fetch result to values")
			}

			llog.Tracef(
				"Account{ totalBalance: %d }",
				totalBalance,
			)
		}
	}

	if queryResult.Err() != nil {
		return nil, merry.Prepend(
			queryResult.Err(),
			"failed to work with response result",
		)
	}

	return inf.NewDec(totalBalance, 0), nil
}

func (ydbCluster *YandexDBCluster) PersistTotal(total inf.Dec) error {
	var err error
	tablePath := path.Join(stroppyDir, "checksum")

	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	writeTX := table.TxControl(
		table.BeginTx(table.WithSerializableReadWrite()),
		table.CommitTx(),
	)

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, _, err = ydbSession.Execute(
				ydbContext,
				writeTX,
				fmt.Sprintf("DECLARE $name AS String;"+
					"DECLARE $amount AS Int64;"+
					"UPSERT INTO `%s` (name, amount) "+
					"VALUES ($name, $amount)",
					tablePath,
				),
				table.NewQueryParameters(
					table.ValueParam("name", types.StringValue([]byte("total"))),
					table.ValueParam("amount", types.Int64Value(total.UnscaledBig().Int64())),
				),
				options.WithKeepInCache(true),
			); err != nil {
				return merry.Prepend(err, "failed to execute upsert")
			}

			return nil
		},
		table.WithIdempotent(),
	); err != nil {
		return merry.Prepend(err, "Error when inserting data into checksum table")
	}
	return nil
}

func (ydbCluster *YandexDBCluster) BootstrapDB(count, seed int) error {
	var err error

	llog.Infof("Creating the folders and tables...")

	ydbContext, cancel := context.WithCancel(context.Background())

	defer cancel()

	prefix := stroppyDir

	if err = createStroppyDirectory(
		ydbContext,
		ydbCluster.ydbConnection,
		prefix,
	); err != nil {
		return merry.Prepend(err, "Error then creating stroppy directory")
	}

	if err = createSettingsTable(
		ydbContext,
		ydbCluster.ydbConnection.Table(),
		prefix,
	); err != nil {
		return merry.Prepend(err, "Error then creating settings table")
	}

	if err = createAccountTable(
		ydbContext,
		ydbCluster.ydbConnection.Table(),
		prefix,
	); err != nil {
		return merry.Prepend(err, "Error then creating account table")
	}

	if err = createTransferTable(
		ydbContext,
		ydbCluster.ydbConnection.Table(),
		prefix,
	); err != nil {
		return merry.Prepend(err, "Error then creating transfer table")
	}

	if err = createChecksumTable(
		ydbContext,
		ydbCluster.ydbConnection.Table(),
		prefix,
	); err != nil {
		return merry.Prepend(err, "Error then creating checksum table")
	}

	if err = upsertSettings(
		ydbContext,
		ydbCluster.ydbConnection.Table(),
		path.Join(prefix, "settings"),
		fmt.Sprintf("%d", count),
		fmt.Sprintf("%d", seed),
	); err != nil {
		return merry.Prepend(err, "Error then inserting settings into settings table")
	}

	return nil
}

// nolint // functions is not same
func createSettingsTable(ydbContext context.Context, ydbClient table.Client, prefix string) error {
	var err error

	if err = recreateTable(
		ydbContext,
		ydbClient,
		path.Join(prefix, "settings"),
		func(ctx context.Context, session table.Session) error {
			return session.CreateTable(
				ydbContext,
				path.Join(prefix, "settings"),
				options.WithColumn("key", types.Optional(types.TypeString)),
				options.WithColumn("value", types.Optional(types.TypeString)),
				options.WithPrimaryKeyColumn("key"),
			)
		},
	); err != nil {
		return merry.Prepend(err, "Error then calling createSettingsTable")
	}

	llog.Infoln("Database table 'settings' successfully created in YDB cluster")

	return nil
}

func createAccountTable(ydbContext context.Context, ydbClient table.Client, prefix string) error {
	var err error

	if err = recreateTable(
		ydbContext,
		ydbClient,
		path.Join(prefix, "account"),
		func(ctx context.Context, session table.Session) error {
			if err = session.CreateTable(
				ctx,
				path.Join(prefix, "account"),
				options.WithColumn("bic", types.Optional(types.TypeString)),
				options.WithColumn("ban", types.Optional(types.TypeString)),
				options.WithColumn("balance", types.Optional(types.TypeInt64)),
				options.WithPrimaryKeyColumn("bic", "ban"),
			); err != nil {
				return merry.Prepend(err, "Error then calling function for creating table")
			}

			return nil
		},
	); err != nil {
		return merry.Prepend(err, "Error then calling createAccountTable")
	}

	llog.Infoln("Database table 'account' successfully created in YDB cluster")

	return nil
}

func createTransferTable(ydbContext context.Context, ydbClient table.Client, prefix string) error {
	var err error

	if err = recreateTable(
		ydbContext,
		ydbClient,
		path.Join(prefix, "transfer"),
		func(ctx context.Context, session table.Session) error {
			if err = session.CreateTable(
				ctx,
				path.Join(prefix, "transfer"),
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
			); err != nil {
				return merry.Prepend(err, "Error then calling function for creating table")
			}

			return nil
		},
	); err != nil {
		return merry.Prepend(err, "Error then createTransferTable")
	}

	llog.Infoln("Database table 'transfer' successfully created in YDB cluster")

	return nil
}

// nolint // functions createChecksumTable and createSettingsTable is not same
func createChecksumTable(ydbContext context.Context, ydbClient table.Client, prefix string) error {
	var err error

	if err = recreateTable(
		ydbContext,
		ydbClient,
		path.Join(prefix, "checksum"),
		func(ctx context.Context, session table.Session) error {
			return session.CreateTable(
				ydbContext,
				path.Join(prefix, "checksum"),
				options.WithColumn("name", types.Optional(types.TypeString)),
				options.WithColumn("amount", types.Optional(types.TypeInt64)),
				options.WithPrimaryKeyColumn("name"),
			)
		},
	); err != nil {
		return merry.Prepend(err, "Error then calling createChecksumTable")
	}

	llog.Infoln("Database table 'checksum' successfully created in YDB cluster")

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
			if err = session.DropTable(
				ctx,
				tablePath,
			); err != nil && strings.Contains(err.Error(), schemeErr) {
				llog.Debugf(
					"Database table '%s' does not exists at this moment in YDB cluster",
					tablePath,
				)
			} else {
				return merry.Prepend(err, "Error inside session.DropTable")
			}

			return nil
		},
		table.WithIdempotent(),
	); err != nil {
		return merry.Prepend(err, fmt.Sprintf("Error then droping '%s' table", tablePath))
	}

	if err = ydbClient.Do(ydbContext, createFunc, table.WithIdempotent()); err != nil {
		return merry.Prepend(err, fmt.Sprintf("Error then creating '%s' table", tablePath))
	}

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
		return merry.Prepend(
			err,
			fmt.Sprintf("Error then creating directory %s in YDB", ydbDirPath),
		)
	}

	llog.Infof("Database directory '%s' successfully created in YDB cluster", ydbDirPath)

	return nil
}

func upsertSettings(
	ydbContext context.Context,
	ydbTableClient table.Client,
	tablePath, count, seed string,
) error {
	var err error

	writeTX := table.TxControl(
		table.BeginTx(table.WithSerializableReadWrite()),
		table.CommitTx(),
	)

	if err = ydbTableClient.Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, _, err = ydbSession.Execute(
				ydbContext,
				writeTX,
				fmt.Sprintf("DECLARE $key AS List<String>;"+
					"DECLARE $value AS List<String>;"+
					"UPSERT INTO `%s` (key, value) "+
					"VALUES ($key[0], $value[0]), ($key[1], $value[1])",
					tablePath,
				),
				table.NewQueryParameters(
					table.ValueParam("key", types.ListValue(
						types.StringValue([]byte("count")),
						types.StringValue([]byte("seed")),
					)),
					table.ValueParam("value", types.ListValue(
						types.StringValue([]byte(count)),
						types.StringValue([]byte(seed)),
					)),
				),
				options.WithKeepInCache(true),
			); err != nil {
				return merry.Prepend(err, "failed to execute upsert")
			}

			return nil
		},
		table.WithIdempotent(),
	); err != nil {
		return merry.Prepend(err, "Error then inserting data in table")
	}

	llog.Infoln("Database table settings successfully inserted")

	return nil
}

func (ydbCluster *YandexDBCluster) InsertAccount(acc model.Account) error {
	var err error

	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	writeTX := table.TxControl(
		table.BeginTx(table.WithSerializableReadWrite()),
		table.CommitTx(),
	)

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, _, err = ydbSession.Execute(
				ydbContext, writeTX, ydbCluster.acc_insert,
				table.NewQueryParameters(
					table.ValueParam("bic", types.StringValue([]byte(acc.Bic))),
					table.ValueParam("ban", types.StringValue([]byte(acc.Ban))),
					table.ValueParam("balance", types.Int64Value(acc.Balance.UnscaledBig().Int64())),
				),
				options.WithKeepInCache(true),
			); err != nil {
				return merry.Prepend(err, "failed to execute upsert")
			}
			return nil
		},
		table.WithIdempotent(),
	); err != nil {
		return merry.Prepend(err, "Error then inserting data in table")
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

func insertEscapedPath(query string, tablePaths ...string) string {
	newTablePaths := make([]interface{}, len(tablePaths))
	for index, tablePath := range tablePaths {
		newTablePaths[index] = "`" + tablePath + "`"
	}

	return fmt.Sprintf(query, newTablePaths...)
}
