package cluster

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/ansel1/merry/v2"
	"github.com/google/uuid"
	llog "github.com/sirupsen/logrus"
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
	schemeErr  string = "Path does not exist"
	stroppyDir string = "stroppy"
)

type YandexDBCluster struct {
	ydbConnection ydb.Connection
}

func NewYandexDBCluster(ydbContext context.Context, dbURL string) (*YandexDBCluster, error) {
	llog.Infof("Establishing connection to YDB on %s", dbURL)

	var (
		database ydb.Connection
		err      error
	)

	if database, err = ydb.Open(ydbContext, dbURL); err != nil {
		return nil, merry.Prepend(err, "Error then creating YDB connection holder")
	}

	return &YandexDBCluster{ydbConnection: database}, nil
}

func (*YandexDBCluster) GetClusterType() DBClusterType {
	return YandexDBClusterType
}

func (ydbCluster *YandexDBCluster) FetchSettings() (Settings, error) {
	var (
		err            error
		clusterSettins Settings
	)

	tablePath := path.Join(ydbCluster.ydbConnection.Name(), stroppyDir, "settings")
	ydbContext, ctxCloseFn := context.WithTimeout(
		context.Background(),
		time.Second,
	)

	defer ctxCloseFn()

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			var queryResult result.StreamResult
			if queryResult, err = ydbSession.StreamReadTable(
				ydbContext,
				tablePath,
			); err != nil {
				return merry.Prepend(err, "failed to fetch rows")
			}

			llog.Infoln("Settings successfully fetched from ydb")

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
	transfer *model.Transfer,
	clientID uuid.UUID,
) error {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) FetchAccounts() ([]model.Account, error) {
	panic("unimplemented!")
}

//nolint:gocritic // two conflicting linters
func (ydbCluster *YandexDBCluster) FetchBalance(
	bic string,
	ban string,
) (*inf.Dec, *inf.Dec, error) {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) FetchTotal() (*inf.Dec, error) {
	var (
		err         error
		queryResult result.Result
		amount      int64
	)

	tablePath := path.Join(ydbCluster.ydbConnection.Name(), stroppyDir, "checksum")
	ydbContext, ctxCloseFn := context.WithTimeout(
		context.Background(),
		time.Second,
	)

	defer ctxCloseFn()

	readTx := table.TxControl(
		table.BeginTx(
			table.WithOnlineReadOnly(),
		),
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

	tablePath := path.Join(ydbCluster.ydbConnection.Name(), stroppyDir, "account")
	ydbContext, ctxCloseFn := context.WithTimeout(
		context.Background(),
		time.Second,
	)

	defer ctxCloseFn()

	readTx := table.TxControl(
		table.BeginTx(
			table.WithOnlineReadOnly(),
		),
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

	tablePath := path.Join(ydbCluster.ydbConnection.Name(), stroppyDir, "checksum")
	ydbContext, ctxCloseFn := context.WithTimeout(
		context.Background(),
		time.Second,
	)

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
			); err != nil {
				return merry.Prepend(err, "failed to execute upsert")
			}

			return nil
		},
	); err != nil {
		return merry.Prepend(err, "Error then inserting data in table")
	}

	return nil
}

func (ydbCluster *YandexDBCluster) BootstrapDB(count, seed int) error {
	var err error

	llog.Infof("Creating the folders and tables...")

	ydbContext, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)

	defer cancel()

	prefix := path.Join(ydbCluster.ydbConnection.Name(), stroppyDir)

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

//nolint // functions is not same
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

//nolint // functions createChecksumTable and createSettingsTable is not same
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
	); err != nil {
		return merry.Prepend(err, fmt.Sprintf("Error then droping '%s' table", tablePath))
	}

	if err = ydbClient.Do(ydbContext, createFunc); err != nil {
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
			); err != nil {
				return merry.Prepend(err, "failed to execute upsert")
			}

			return nil
		},
	); err != nil {
		return merry.Prepend(err, "Error then inserting data in table")
	}

	llog.Infoln("Database table settings successfully inserted")

	return nil
}

func (ydbCluster *YandexDBCluster) InsertAccount(acc model.Account) error {
	var err error

	writeTX := table.TxControl(
		table.BeginTx(table.WithSerializableReadWrite()),
		table.CommitTx(),
	)

	tablePath := path.Join(ydbCluster.ydbConnection.Name(), stroppyDir, "account")
	ydbContext, ctxCloseFn := context.WithTimeout(
		context.Background(),
		time.Second,
	)

	defer ctxCloseFn()

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			if _, _, err = ydbSession.Execute(
				ydbContext,
				writeTX,
				fmt.Sprintf("DECLARE $bic AS String;"+
					"DECLARE $ban AS String;"+
					"DECLARE $balance AS Int64;"+
					"UPSERT INTO `%s` (bic, ban, balance) VALUES ($bic, $ban, $balance)",
					tablePath,
				),
				table.NewQueryParameters(
					table.ValueParam("bic", types.StringValue([]byte(acc.Bic))),
					table.ValueParam("ban", types.StringValue([]byte(acc.Ban))),
					table.ValueParam("balance", types.Int64Value(acc.Balance.UnscaledBig().Int64())),
				),
			); err != nil {
				return merry.Prepend(err, "failed to execute upsert")
			}

			return nil
		},
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