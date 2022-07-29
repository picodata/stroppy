package cluster

import (
	"context"
	"fmt"
	"path"

	"github.com/ansel1/merry/v2"
	llog "github.com/sirupsen/logrus"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/table"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/options"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"
)

const STROPPY_DIR string = "stroppy"

type YandexDBCluster struct {
	ydbConnection ydb.Connection
    ydbContext context.Context
}

func NewYandexDBCluster(DBUrl string) (*YandexDBCluster, error) {
	llog.Infof("Establishing connection to YDB on %s", DBUrl)

	var (
		database ydb.Connection
		err      error
	)

	ydbContext, cancel := context.WithCancel(context.Background())
	
    if database, err = ydb.Open(ydbContext, DBUrl, ydb.WithAnonymousCredentials()); err != nil {
		return nil, merry.Prepend(err, "Error then creating YDB connection holder")
	}

	defer func() { 
        _ = database.Close(ydbContext)
        cancel()
    }()

    return &YandexDBCluster{ydbConnection: database, ydbContext: ydbContext}, nil
}

//nolint // will be fixed with ydb crud mr
func (ydb *YandexDBCluster) BootstrapDB(count, seed int) error {
	var err error

	llog.Infof("Creating the folders and tables...")

	prefix := path.Join(ydb.ydbConnection.Name(), STROPPY_DIR)

	if err = createStroppyDirectory(ydb.ydbContext, ydb.ydbConnection, prefix); err != nil {
		panic(fmt.Sprintf("Error then creating stroppy directory: %s", err))
	}

	if err = createSettingsTable(ydb.ydbContext, ydb.ydbConnection.Table(), prefix); err != nil {
		panic(fmt.Sprintf("Error then creating settings table: %s", err))
	}

	if err = createAccountTable(ydb.ydbContext, ydb.ydbConnection.Table(), prefix); err != nil {
		panic(fmt.Sprintf("Error then account settings table: %s", err))
	}

	if err = createTransferTable(ydb.ydbContext, ydb.ydbConnection.Table(), prefix); err != nil {
		panic(fmt.Sprintf("Error then transfer settings table: %s", err))
	}

	if err = createChecksumTable(ydb.ydbContext, ydb.ydbConnection.Table(), prefix); err != nil {
		panic(fmt.Sprintf("Error then checksum settings table: %s", err))
	}

	return nil
}


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

	llog.Infoln("Table options succeefully described")

	return nil
}

func createAccountTable(ydbContext context.Context, ydbClient table.Client, prefix string) error {
	var err error

	if err = recreateTable(
		ydbContext,
		ydbClient,
		path.Join(prefix, "account"),
		func(ctx context.Context, session table.Session) error {
			return session.CreateTable(
				ydbContext,
				path.Join(prefix, "account"),
				options.WithColumn("bic", types.Optional(types.TypeString)),
				options.WithColumn("ban", types.Optional(types.TypeString)),
				options.WithColumn("balance", types.Optional(types.TypeInt64)),
				options.WithPrimaryKeyColumn("bic", "ban"),
			)
		},
	); err != nil {
		return merry.Prepend(err, "Error then calling createAccountTable")
	}

	llog.Infoln("Settings table succeefully created")

	return nil
}

func createTransferTable(ydbContext context.Context, ydbClient table.Client, prefix string) error {
	var err error

	if err = recreateTable(
		ydbContext,
		ydbClient,
		path.Join(prefix, "transfer"),
		func(ctx context.Context, session table.Session) error {
			return session.CreateTable(
				ydbContext,
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
			)
		},
	); err != nil {
		return merry.Prepend(err, "Error then createTransferTable")
	}

	llog.Infoln("Account table succeefully created")

	return nil
}

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

	llog.Infoln("Account table succeefully created")

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
			return session.DropTable(ydbContext, tablePath)
		},
	); err != nil {
		merry.Prepend(err, fmt.Sprintf("Error then droping '%s' table", tablePath))
	}

	if err = ydbClient.Do(ydbContext, createFunc); err != nil {
		return merry.Prepend(err, fmt.Sprintf("Error then creating '%s' table", tablePath))
	}

	return nil
}

func createStroppyDirectory(
	ydbContext context.Context,
	ydbConnection ydb.Connection,
	prefix string,
) error {
	if err := ydbConnection.Scheme().RemoveDirectory(
		ydbContext,
		path.Join(prefix, STROPPY_DIR),
	); err != nil {
		llog.Debug("Directory %s does not exists in YDB cluster", path.Join(prefix, STROPPY_DIR))
	}

	if err := ydbConnection.Scheme().MakeDirectory(
		ydbContext,
		path.Join(prefix, STROPPY_DIR),
	); err != nil {
		return merry.Prepend(
			err,
			fmt.Sprintf("Error then creating directory %s in YDB", path.Join(prefix, STROPPY_DIR)),
		)
	}

	llog.Infoln("Directory '%s' succeefully created")

	return nil
}

func (*YandexDBCluster) GetClusterType() DBClusterType {
	return YandexDBClusterType
}
