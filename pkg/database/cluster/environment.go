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
	defaultCockroachDBUrl     = "postgresql://root@Raider:26257/defaultdb?sslmode=disable"
)

func GetEnvDataStore(opts string) (dbParams string, err error) {
	var present bool
	switch opts {
	case Foundation:
		return "", merry.Errorf("unsupported store type %s", opts)
	case Postgres:
		return "", merry.Errorf("unsupported store type %s", opts)
	case MongoDB:
		dbParams, present = os.LookupEnv("TEST_MONGODB_STR")
		if !present {
			dbParams = defaultMongoDBUrl
		}
	case Cockroach:
		dbParams, present = os.LookupEnv("TEST_COCKROACHDB_STR")
		if !present {
			dbParams = defaultCockroachDBUrl
		}
	case Cartridge:
		return "", merry.Errorf("unsupported store type %s", opts)
	default:
		return "", merry.Errorf("unsupported store type %s", opts)
	}
	return dbParams, nil
}
