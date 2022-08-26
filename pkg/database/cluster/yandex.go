package cluster

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/ansel1/merry/v2"
	"github.com/google/uuid"
	llog "github.com/sirupsen/logrus"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/table"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/options"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"
	"gitlab.com/picodata/stroppy/internal/model"
	"gopkg.in/inf.v0"
)

const (
	schemeErr  string = "Path does not exist"
	stroppyDir string = "stroppy"
)

type YandexDBCluster struct {
	conn ydb.Connection
}

//nolint
func NewYandexDBCluster(ydbContext context.Context, dbURL string) (*YandexDBCluster, error) {
	llog.Infof("Establishing connection to YDB on %s", dbURL)

	var (
		database ydb.Connection
		err      error
	)

	if database, err = ydb.Open(ydbContext, dbURL); err != nil {
		//nolint:wrapcheck // linter does not understand the error wrapped from external library
		return nil, merry.Prepend(err, "Error then creating YDB connection holder")
	}

	return &YandexDBCluster{conn: database}, nil
}

func (*YandexDBCluster) GetClusterType() DBClusterType {
	return YandexDBClusterType
}

func (ydbCluster *YandexDBCluster) FetchSettings() (Settings, error) {
	panic("unimplemented!")
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
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) CheckBalance() (*inf.Dec, error) {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) PersistTotal(total inf.Dec) error {
	panic("unimplemented!")
}

func (ydbCluster *YandexDBCluster) BootstrapDB(count, seed int) error {
	var err error

	llog.Infof("Creating the folders and tables...")

	ydbContext, cancel := context.WithCancel(context.Background())

	defer cancel()

	prefix := path.Join(ydbCluster.conn.Name(), stroppyDir)

	if err = createStroppyDirectory(
		ydbContext,
		ydbCluster.conn,
		prefix,
	); err != nil {
		panic(fmt.Sprintf("Error then creating stroppy directory: %s", err))
	}

	if err = createSettingsTable(
		ydbContext,
		ydbCluster.conn.Table(),
		prefix,
	); err != nil {
		panic(fmt.Sprintf("Error then creating settings table: %s", err))
	}

	if err = createAccountTable(
		ydbContext,
		ydbCluster.conn.Table(),
		prefix); err != nil {
		panic(fmt.Sprintf("Error then account settings table: %s", err))
	}

	if err = createTransferTable(
		ydbContext,
		ydbCluster.conn.Table(),
		prefix,
	); err != nil {
		panic(fmt.Sprintf("Error then transfer settings table: %s", err))
	}

	if err = createChecksumTable(
		ydbContext,
		ydbCluster.conn.Table(),
		prefix,
	); err != nil {
		panic(fmt.Sprintf("Error then checksum settings table: %s", err))
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
				//nolint:wrapcheck // linter does not understand then wrapped from external library
				return merry.Prepend(err, "Error then calling function for creating table")
			}

			return nil
		},
	); err != nil {
		//nolint:wrapcheck // linter does not understand then error wrapped from external library
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
				//nolint:wrapcheck // linter does not understand then wrapped from external library
				return merry.Prepend(err, "Error then calling function for creating table")
			}

			return nil
		},
	); err != nil {
		//nolint:wrapcheck // linter does not understand then error wrapped from external library
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
				//nolint:wrapcheck // linter does not understand then wrapped from external library
				return merry.Prepend(err, "Error inside session.DropTable")
			}

			return nil
		},
	); err != nil {
		//nolint:wrapcheck // linter does not understand then error wrapped from external library
		return merry.Prepend(err, fmt.Sprintf("Error then droping '%s' table", tablePath))
	}

	if err = ydbClient.Do(ydbContext, createFunc); err != nil {
		//nolint:wrapcheck // linter does not understand then error wrapped from external library
		return merry.Prepend(err, fmt.Sprintf("Error then creating '%s' table", tablePath))
	}

	return nil
}

func createStroppyDirectory(
	ydbContext context.Context,
	conn ydb.Connection,
	ydbDirPath string,
) error {
	if err := conn.Scheme().RemoveDirectory(
		ydbContext,
		ydbDirPath,
	); err != nil {
		llog.Debugf("Database directory '%s' does not exists in YDB cluster", ydbDirPath)
	}

	if err := conn.Scheme().MakeDirectory(
		ydbContext,
		ydbDirPath,
	); err != nil {
		//nolint:wrapcheck // linter does not understand then error wrapped from external library
		return merry.Prepend(
			err,
			fmt.Sprintf("Error then creating directory %s in YDB", ydbDirPath),
		)
	}

	llog.Infof("Database directory '%s' successfully created in YDB cluster", ydbDirPath)

	return nil
}

func (ydbCluster *YandexDBCluster) InsertAccount(acc model.Account) error {
	panic("unimplemented!")
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
