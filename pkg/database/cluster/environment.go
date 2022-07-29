package cluster

import (
	"os"

	"github.com/ansel1/merry"
)

const (
	poolSize                  = 128
	defaultMongoDBUrl         = "mongodb://127.0.0.1:27017/stroppy"
	expectedCount             = 10000
	defaultBanRangeMultiplier = 1.1
	defaultCockroachDBUrl     = "postgresql://root@localhost:26257/defaultdb?sslmode=disable"
	defaultPostgresDBUrl      = "postgresql://stroppy:stroppy@localhost/stroppy?sslmode=disable"
	defaultFoundationDBUrl    = "/etc/foundationdb/fdb.cluster"
	defaultYandexDBUrl        = "grpc://localhost:2136/local" // TODO: secure connection.

)

func GetEnvDataStore(opts string) (dbParams string, err error) {
	var present bool
	switch opts {
	case Foundation:
		dbParams, present = os.LookupEnv("TEST_FDB_URL")
		if !present {
			dbParams = defaultFoundationDBUrl
		}
	case Postgres:
		dbParams, present = os.LookupEnv("TEST_POSTGRES_URL")
		if !present {
			dbParams = defaultPostgresDBUrl
		}
	case MongoDB:
		dbParams, present = os.LookupEnv("TEST_MONGODB_URL")
		if !present {
			dbParams = defaultMongoDBUrl
		}
	case Cockroach:
		dbParams, present = os.LookupEnv("TEST_COCKROACHDB_URL")
		if !present {
			dbParams = defaultCockroachDBUrl
		}
	case Cartridge:
		return "", merry.Errorf("unsupported store type %s", opts)
	case YandexDB:
		if dbParams, present = os.LookupEnv("TEST_YDB_URL"); !present {
			dbParams = defaultYandexDBUrl
		}
	default:
		return "", merry.Errorf("unsupported store type %s", opts)
	}

	return dbParams, nil
}
