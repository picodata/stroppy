package cluster

import (
	"errors"
)

// ErrConsistencyViolation should be returned if tx is not safe to retry and data could be damaged.
var ErrConsistencyViolation = errors.New("cluster: data consistency may be violated")

// ErrInsufficientFunds is that probably shouldn't be here,
// but it simplifies general payment logic so let it stay until we find another place for it.
var ErrInsufficientFunds = errors.New("cluster: insufficient funds for transfer")

// ErrTxRollback should be returned if tx was rollbacked and is safe to retry.
var ErrTxRollback = errors.New("cluster: transaction rollback")

// ErrNoRows is a general 'not found' err, to abstract from sql.ErrNoRows.
var ErrNoRows = errors.New("cluster: no rows in result set")

// ErrTimeoutExceeded is ...TODO: transform into any transient error.
var ErrTimeoutExceeded = errors.New("cluster: query timeout exceeded")

// ErrDuplicateKey is returned then there already such unique key
var ErrDuplicateKey = errors.New("cluster: duplicate unique key")

// DBClusterType is type for choose ClusterType.
type DBClusterType int

// PostgresClusterType is constant for save type PostgresCluster.
const (
	PostgresClusterType DBClusterType = iota
	FDBClusterType
)

func (e DBClusterType) String() string {
	switch e {
	case PostgresClusterType:
		return "PostgreSQL"
	case FDBClusterType:
		return "FoundationDB"
	}
	panic("unknown DBClusterType")
}

const PostgresType = "postgres"

const FDBType = "fdb"

// ClusterSettings returns the test run settings.
type ClusterSettings struct {
	Count int
	Seed  int
}