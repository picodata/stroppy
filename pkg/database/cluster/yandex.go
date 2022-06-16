package cluster

import (
	"context"
	"path"

	"github.com/ansel1/merry/v2"
	llog "github.com/sirupsen/logrus"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/table"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/options"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"
)

type YandexDBCluster struct {
	db ydb.Connection
}

//nolint // ydb cluster creation in future
func NewYandexDBCluster(dbURL string, _ int) (*YandexDBCluster, error) {
	llog.Infof("Establishing connection to YDB on %v", dbURL)

	var (
		database ydb.Connection
		err      error
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if database, err = ydb.New(ctx, ydb.WithAnonymousCredentials()); err != nil {
		return nil, merry.Prepend(err, "Error then creating YDB driver runtime holder")
	}

	return &YandexDBCluster{
		database,
	}, nil
}

func (*YandexDBCluster) GetClusterType() DBClusterType {
	return YandexDBClusterType
}

//nolint // will be fixed with ydb crud mr
func (y *YandexDBCluster) BootstrapDB(count, seed int) error {
	var err error

	llog.Infof("Creating the tables...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err = y.db.Table().Do(
		ctx,
		func(ctx context.Context, s table.Session) error {
			if err = s.CreateTable(ctx, path.Join(y.db.Name(), "series"),
				options.WithColumn("series_id", types.Optional(types.TypeUint64)),
				options.WithColumn("title", types.Optional(types.TypeUTF8)),
				options.WithColumn("series_info", types.Optional(types.TypeUTF8)),
				options.WithColumn("release_date", types.Optional(types.TypeDate)),
				options.WithColumn("comment", types.Optional(types.TypeUTF8)),
				options.WithPrimaryKeyColumn("series_id"),
			); err != nil {
				return merry.Prepend(err, "Error then creating YDB table")
			}

			return nil
		},
	); err != nil {
		return merry.Prepend(err, "Error then creating YDB table")
	}

	return nil
}
