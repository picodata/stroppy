package cluster

import (
	"context"
	"crypto/sha1" //nolint
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

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
	"gopkg.in/inf.v0"

	"gitlab.com/picodata/stroppy/internal/model"
)

const (
	schemeErr    string = "Path does not exist"
	stroppyDir   string = "stroppy"
	stroppyAgent string = "stroppy 1.0"
	// default operation timeout.
	defaultTimeout = time.Second * 10
	// extra connections in the pool.
	poolSizeOverhead = 10
)

var (
	//go:embed ydb_insert_account.yql
	yqlInsertAccount string

	//go:embed ydb_transfer.yql
	yqlTransfer string

	//go:embed ydb_select_balance_account.yql
	yqlSelectBalanceAccount string
)

type YdbCluster struct {
	ydbConnection       ydb.Connection
	yqlInsertAccount    string
	yqlSelectBalanceAcc string
	yqlTransferSingleOp string
	transferIDHashing   bool
	partitionsMaxSize   int
	partitionsMinCount  int
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

func envTransferIDHashing() bool {
	if value, ok := os.LookupEnv("YDB_STROPPY_HASH_TRANSFER_ID"); ok {
		if (value == "1") || (value == "Y") {
			llog.Infoln("YDB transfer id hashing is ENABLED")

			return true
		}
	}

	return false
}

func envPartitionsMinCount() int {
	ret := 300

	if value, ok := os.LookupEnv("YDB_STROPPY_PARTITIONS_COUNT"); ok {
		partitionNum, err := strconv.Atoi(value)
		if err != nil || partitionNum <= 0 || partitionNum > 10000 {
			llog.Warningln(
				"Illegal value [",
				value,
				"] passed in YDB_STROPPY_PARTITIONS_COUNT, ignored",
			)
		} else {
			ret = partitionNum
		}
	}

	llog.Infoln("Using YDB minimal partition count ", ret)

	return ret
}

func envPartitionsMaxSize() int {
	ret := 512

	if value, ok := os.LookupEnv("YDB_STROPPY_PARTITIONS_SIZE"); ok {
		partitionNum, err := strconv.Atoi(value)
		if err != nil || partitionNum <= 0 || partitionNum > 10000 {
			llog.Warningln(
				"Illegal value [",
				value,
				"] passed in YDB_STROPPY_PARTITIONS_SIZE, ignored",
			)
		} else {
			ret = partitionNum
		}
	}

	llog.Infoln("Using YDB maximal partition size ", ret)

	return ret
}

func envTLSCertificateFile() string {
	if value, ok := os.LookupEnv("YDB_TLS_CERTIFICATES_FILE"); ok {
		return value
	}

	return ""
}

func NewYdbCluster(
	ydbContext context.Context,
	dbURL string,
	poolSize uint64,
) (*YdbCluster, error) {
	llog.Infof("YDB Go SDK version %s", ydb.Version)
	llog.Infof("Establishing connection to YDB on %s with poolSize %d", dbURL, poolSize)

	var (
		database ydb.Connection
		err      error
	)

	ydbOptions := []ydb.Option{
		ydb.WithUserAgent(stroppyAgent),
		ydb.WithSessionPoolSizeLimit(int(poolSize + poolSizeOverhead)),
		ydb.WithSessionPoolIdleThreshold(defaultTimeout),
		ydb.WithDiscoveryInterval(defaultTimeout),
	}
	if envConfigured() {
		llog.Infoln("YDB connection credentials are configured through the environment")

		ydbOptions = append(ydbOptions, environ.WithEnvironCredentials(ydbContext))
	}

	if tlsCertFile := envTLSCertificateFile(); len(tlsCertFile) > 0 {
		llog.Infoln("YDB custom TLS certificate file: ", tlsCertFile)
		ydbOptions = append(ydbOptions, ydb.WithCertificatesFromFile(tlsCertFile))
	}

	database, err = ydb.Open(ydbContext, dbURL, ydbOptions...)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create YDB connection")
	}

	return &YdbCluster{
		ydbConnection:       database,
		yqlInsertAccount:    expandYql(yqlInsertAccount),
		yqlSelectBalanceAcc: expandYql(yqlSelectBalanceAccount),
		yqlTransferSingleOp: expandYql(yqlTransfer),
		transferIDHashing:   envTransferIDHashing(),
		partitionsMaxSize:   envPartitionsMaxSize(),
		partitionsMinCount:  envPartitionsMinCount(),
	}, nil
}

func (*YdbCluster) GetClusterType() DBClusterType {
	return YandexDBClusterType
}

var (
	globalYdbClusterSettings    *Settings
	globalYdbClusterSettingsMtx sync.Mutex
)

func (ydbCluster *YdbCluster) FetchSettings() (Settings, error) {
	globalYdbClusterSettingsMtx.Lock()
	defer globalYdbClusterSettingsMtx.Unlock()

	if globalYdbClusterSettings != nil {
		return *globalYdbClusterSettings, nil
	}

	var (
		err             error
		clusterSettings Settings
	)

	defer func() {
		if err == nil {
			globalYdbClusterSettings = &clusterSettings
		}
	}()

	ydbContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	selectStmnt := fmt.Sprintf("SELECT key, value FROM `%s/settings`", stroppyDir)

	if err = ydbCluster.ydbConnection.Table().Do(
		ydbContext,
		func(ydbContext context.Context, ydbSession table.Session) error {
			var rows result.Result
			_, rows, err = ydbSession.Execute(ydbContext, table.DefaultTxControl(), selectStmnt, nil)
			if err != nil {
				return errors.Wrap(err, "failed to execute query")
			}
			defer func() {
				_ = rows.Close()
			}()

			var (
				key   string
				value string
			)
			for rows.NextResultSet(ydbContext) {
				for rows.NextRow() {
					if err = rows.ScanNamed(
						named.OptionalWithDefault("key", &key),
						named.OptionalWithDefault("value", &value),
					); err != nil {
						return errors.Wrap(err, "failed to get next row in scan")
					}
					llog.Tracef("Settings{ key: %s, value: %s }", key, value)
					switch key {
					case "count":
						if clusterSettings.Count, err = strconv.Atoi(value); err != nil {
							return errors.Wrap(err, "failed to convert count into integer")
						}
					case "seed":
						if clusterSettings.Seed, err = strconv.Atoi(value); err != nil {
							return errors.Wrap(err, "failed to convert seed into integer")
						}
					}
				}
			}

			return nil
		},
		table.WithIdempotent(),
	); err != nil {
		return clusterSettings, errors.Wrap(err, "Error fetching data from settings table")
	}

	return clusterSettings, nil
}

func transferID(useHash bool, transferId *model.TransferId) string { //nolint
	if useHash {
		hasher := sha1.New() //nolint
		hasher.Write(transferId[:])

		return base64.URLEncoding.EncodeToString(hasher.Sum(nil))
	}

	return transferId.String()
}

func (ydbCluster *YdbCluster) MakeAtomicTransfer(
	transfer *model.Transfer, //nolint
	clientID uuid.UUID,
) error {
	var err error

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	amount := transfer.Amount.UnscaledBig().Int64()
	transferID := transferID(ydbCluster.transferIDHashing, &transfer.Id)

	// Execute the single-statement transfer transaction
	if err = ydbCluster.ydbConnection.Table().Do(ctx,
		func(ctx context.Context, s table.Session) error {
			_, _, err = s.Execute(ctx,
				table.SerializableReadWriteTxControl(table.CommitTx()),
				ydbCluster.yqlTransferSingleOp,
				table.NewQueryParameters(
					table.ValueParam("transfer_id",
						types.BytesValueFromString(transferID)),
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
				// TODO: find a better way to grab the specific errors
				text := err.Error()
				if strings.Contains(text, "MISSING_ACCOUNTS") {
					llog.Tracef(
						"missing transfer: src_bic: %s, src_ban: %s dst_bic: %s, dst_ban: %s",
						transfer.Acs[0].Bic, transfer.Acs[0].Ban,
						transfer.Acs[1].Bic, transfer.Acs[1].Ban,
					)
					return ErrNoRows
				}
				if strings.Contains(text, "INSUFFICIENT_FUNDS") {
					llog.Tracef(
						"insufficient funds: src_bic: %s, src_ban: %s dst_bic: %s, dst_ban: %s",
						transfer.Acs[0].Bic, transfer.Acs[0].Ban,
						transfer.Acs[1].Bic, transfer.Acs[1].Ban,
					)

					return ErrInsufficientFunds
				}

				return errors.Wrap(err, "failed to execute the transfer")
			}

			return nil
		},
		// Mark the operation idempotent to allow retries.
		table.WithIdempotent(),
	); err != nil {
		return errors.Wrap(err, "failed to execute query")
	}

	return nil
}

func (ydbCluster *YdbCluster) FetchAccounts() ([]model.Account, error) {
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
				return errors.Wrap(err, "failed to execute scan query on account table")
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

func (ydbCluster *YdbCluster) FetchBalance( //nolint
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

func (ydbCluster *YdbCluster) FetchTotal() (*inf.Dec, error) {
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

func (ydbCluster *YdbCluster) CheckBalance() (*inf.Dec, error) {
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

func (ydbCluster *YdbCluster) PersistTotal(total inf.Dec) error {
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

func (ydbCluster *YdbCluster) BootstrapDB(count uint64, seed int) error {
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

	if err = ydbCluster.createSettingsTable(ydbContext, prefix); err != nil {
		return errors.Wrap(err, "failed to bootstrap yandex database")
	}

	if err = ydbCluster.createAccountTable(ydbContext, prefix); err != nil {
		return errors.Wrap(err, "failed to bootstrap yandex database")
	}

	if err = ydbCluster.createTransferTable(ydbContext, prefix); err != nil {
		return errors.Wrap(err, "failed to bootstrap yandex database")
	}

	if err = ydbCluster.createChecksumTable(ydbContext, prefix); err != nil {
		return errors.Wrap(err, "failed to bootstrap yandex database")
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

func (ydbCluster *YdbCluster) createSettingsTable( //nolint:dupl // because it golang
	ydbContext context.Context,
	prefix string,
) error {
	var err error

	tabname := path.Join(prefix, "settings")
	if err = recreateTable(
		ydbContext, ydbCluster.ydbConnection.Table(), tabname,
		func(ctx context.Context, session table.Session) error {
			if err = session.CreateTable(
				ctx, tabname,
				options.WithColumn("key", types.Optional(types.TypeString)),
				options.WithColumn("value", types.Optional(types.TypeString)),
				options.WithPrimaryKeyColumn("key"),
			); err != nil {
				return errors.Wrap(err, "failed to create settings table")
			}
			return nil
		},
	); err != nil {
		return errors.Wrap(err, "failed to (re)create settings table")
	}

	return nil
}

func (ydbCluster *YdbCluster) createAccountTable(
	ydbContext context.Context,
	prefix string,
) error {
	var err error

	partitionsMinCount := ydbCluster.partitionsMinCount
	if partitionsMinCount < 10 { //nolint
		partitionsMinCount = 10
	} else if partitionsMinCount > 10000 { //nolint
		partitionsMinCount = 10000
	}

	partitionsMaxCount := partitionsMinCount + 10 + (ydbCluster.partitionsMinCount / 10) //nolint

	tabname := path.Join(prefix, "account")
	if err = recreateTable(
		ydbContext, ydbCluster.ydbConnection.Table(), tabname,
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
					options.WithMinPartitionsCount(uint64(partitionsMinCount)),
					options.WithMaxPartitionsCount(uint64(partitionsMaxCount)),
					options.WithPartitionSizeMb(uint64(ydbCluster.partitionsMaxSize)),
				),
			); err != nil {
				return errors.Wrapf(err, "failed to create account table")
			}
			return nil
		},
	); err != nil {
		return errors.Wrap(err, "failed to (re)create account table")
	}

	return nil
}

func (ydbCluster *YdbCluster) createTransferTable(
	ydbContext context.Context,
	prefix string,
) error {
	var err error

	partitionsMinCount := ydbCluster.partitionsMinCount
	if partitionsMinCount < 10 { //nolint
		partitionsMinCount = 10
	} else if partitionsMinCount > 10000 { //nolint
		partitionsMinCount = 10000
	}
	partitionsMaxCount := partitionsMinCount + 10 + (ydbCluster.partitionsMinCount / 10) //nolint

	tabname := path.Join(prefix, "transfer")
	if err = recreateTable(
		ydbContext, ydbCluster.ydbConnection.Table(), tabname,
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
					options.WithMinPartitionsCount(uint64(partitionsMinCount)),
					options.WithMaxPartitionsCount(uint64(partitionsMaxCount)),
					options.WithPartitionSizeMb(uint64(ydbCluster.partitionsMaxSize)),
				),
			); err != nil {
				return errors.Wrapf(err, "failed to create transfer table")
			}
			return nil
		},
	); err != nil {
		return errors.Wrap(err, "failed to (re)create transfer table")
	}

	return nil
}

func (ydbCluster *YdbCluster) createChecksumTable( //nolint:dupl // because it golang
	ydbContext context.Context,
	prefix string,
) error {
	var err error

	tabname := path.Join(prefix, "checksum")
	if err = recreateTable(
		ydbContext, ydbCluster.ydbConnection.Table(), tabname,
		func(ctx context.Context, session table.Session) error {
			if err = session.CreateTable(
				ctx, tabname,
				options.WithColumn("name", types.Optional(types.TypeString)),
				options.WithColumn("amount", types.Optional(types.TypeInt64)),
				options.WithPrimaryKeyColumn("name"),
			); err != nil {
				return errors.Wrapf(err, "failed to create checksum table")
			}

			return nil
		},
	); err != nil {
		return errors.Wrap(err, "failed to (re)create checksum table")
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

func (ydbCluster *YdbCluster) InsertAccount(acc model.Account) error {
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

func (ydbCluster *YdbCluster) InsertTransfer(transfer *model.Transfer) error {
	panic("unimplemented!")
}

func (ydbCluster *YdbCluster) DeleteTransfer(
	transferID model.TransferId,
	clientID uuid.UUID,
) error {
	panic("unimplemented!")
}

func (ydbCluster *YdbCluster) SetTransferClient(
	clientID uuid.UUID,
	transferID model.TransferId,
) error {
	panic("unimplemented!")
}

func (ydbCluster *YdbCluster) FetchTransferClient(
	transferID model.TransferId,
) (*uuid.UUID, error) {
	panic("unimplemented!")
}

func (ydbCluster *YdbCluster) ClearTransferClient(
	transferID model.TransferId,
	clientID uuid.UUID,
) error {
	panic("unimplemented!")
}

func (ydbCluster *YdbCluster) SetTransferState(
	state string,
	transferID model.TransferId,
	clientID uuid.UUID,
) error {
	panic("unimplemented!")
}

func (ydbCluster *YdbCluster) FetchTransfer(
	transferID model.TransferId,
) (*model.Transfer, error) {
	panic("unimplemented!")
}

func (ydbCluster *YdbCluster) FetchDeadTransfers() ([]model.TransferId, error) {
	panic("unimplemented!")
}

func (ydbCluster *YdbCluster) UpdateBalance(
	balance *inf.Dec,
	bic string,
	ban string,
	transferID model.TransferId,
) error {
	panic("unimplemented!")
}

func (ydbCluster *YdbCluster) LockAccount(
	transferID model.TransferId,
	pendingAmount *inf.Dec,
	bic string,
	ban string,
) (*model.Account, error) {
	panic("unimplemented!")
}

func (ydbCluster *YdbCluster) UnlockAccount(
	bic string,
	ban string,
	transferID model.TransferId,
) error {
	panic("unimplemented!")
}

// TODO: check possibility of collecting statistics for YDB.
func (ydbCluster *YdbCluster) StartStatisticsCollect(_ time.Duration) error {
	llog.Debugln("statistic for YDB not implemeted yet, watch grafana metrics, please")

	return nil
}

// Template for generating YQL queries.
var ydbYqlTemplate = template.New("").Funcs(template.FuncMap{
	"stroppyDir": func() string {
		return stroppyDir
	},
})

// Substitute directory path into the YQL template,
// replacing the double quote characters with backticks.
func expandYql(query string) string {
	var buffer strings.Builder
	if err := template.Must(ydbYqlTemplate.Parse(query)).Execute(&buffer, nil); err != nil {
		panic(err)
	}

	return buffer.String()
}
