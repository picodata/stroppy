package store

import (
	"errors"
)

// ErrTxRollback should be returned if tx is not safe to retry and data could be damaged
var ErrConsistencyViolation = errors.New("cluster: data consistency may be violated")

// that probably shouldn't be here, but it simplifies general payment logic so let it stay until we find another place for it
var ErrInsufficientFunds = errors.New("cluster: insufficient funds for transfer")

// ErrTxRollback should be returned if tx was rollbacked and is safe to retry
var ErrTxRollback = errors.New("cluster: transaction rollback")

// ErrNoRows is a general 'not found' err, to abstract from sql.ErrNoRows
var ErrNoRows = errors.New("cluster: no rows in result set")

// TODO: transform into any transient error
var ErrTimeoutExceeded = errors.New("cluster: query timeout exceeded")

type DBClusterType int

const (
	PostgresClusterType DBClusterType = iota
)

func (e DBClusterType) String() string {
	switch e {
	case PostgresClusterType:
		return "PostgreSQL"
	}
	panic("unknown DBClusterType")
}

type ClusterSettings struct {
	Count int
	Seed  int
}
