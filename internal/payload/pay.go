package payload

import (
	"runtime"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
)

// IsTransientError is a wrapper to determine if request was
// terminated due to data inconsistency / logical bug
// or it was just a request / tx timeout etc.
func IsTransientError(err error) bool {
	err = merry.Unwrap(err)

	return err == cluster.ErrTimeoutExceeded
}

var nClients uint64

type PayStats struct {
	errors            uint64
	NoSuchAccount     uint64
	InsufficientFunds uint64
	retries           uint64
	recoveries        uint64
}

func (p *BasePayload) Pay(_ string) (err error) {

	llog.Infof("Establishing connection to the cluster")

	llog.Infof("Making %d transfers using %d workers on %d cores \n",
		p.config.Count, p.config.Workers, runtime.NumCPU())

	if err = p.chaos.ExecuteCommand(p.chaosParameter); err != nil {
		llog.Errorf("failed to execute chaos command: %v", err)
	}

	var payStats *PayStats
	if payStats, err = p.payFunc(p.config, p.Cluster, p.oracle); err != nil {
		return merry.Prepend(err, "pay function failed")
	}
	p.chaos.Stop()

	llog.Infof("Errors: %v, Retries: %v, Recoveries: %v, Not found: %v, Overdraft: %v\n",
		payStats.errors,
		payStats.retries,
		payStats.recoveries,
		payStats.NoSuchAccount,
		payStats.InsufficientFunds)

	return
}
