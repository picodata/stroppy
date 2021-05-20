package funcs

import (
	"gitlab.com/picodata/stroppy/benchmark/pkg/database"
	"gitlab.com/picodata/stroppy/benchmark/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/benchmark/pkg/database/config"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/picodata/stroppy/benchmark/internal/fixed_random_source"
	"gitlab.com/picodata/stroppy/benchmark/internal/model"
	"gitlab.com/picodata/stroppy/benchmark/pkg/statistics"

	"github.com/ansel1/merry"
	"github.com/google/uuid"
	llog "github.com/sirupsen/logrus"
	"gopkg.in/inf.v0"
)

// CustomTxTransfer
// This interface is used to implement money transfer mechanism with
// custom locking on the app level.
// It is much more complicated and should only be used for dbs without builtin ACID transactions.
//
// Should also satisfy PredictableCluster interface
type CustomTxTransfer interface {
	GetClusterType() cluster.DBClusterType
	FetchSettings() (cluster.ClusterSettings, error)

	InsertTransfer(transfer *model.Transfer) error
	DeleteTransfer(transferId model.TransferId, clientId uuid.UUID) error
	SetTransferClient(clientId uuid.UUID, transferId model.TransferId) error
	FetchTransferClient(transferId model.TransferId) (*uuid.UUID, error)
	ClearTransferClient(transferId model.TransferId, clientId uuid.UUID) error
	SetTransferState(state string, transferId model.TransferId, clientId uuid.UUID) error
	FetchTransfer(transferId model.TransferId) (*model.Transfer, error)
	FetchDeadTransfers() ([]model.TransferId, error)

	UpdateBalance(balance *inf.Dec, bic string, ban string, transferId model.TransferId) error

	LockAccount(transferId model.TransferId, pendingAmount *inf.Dec, bic string, ban string) (*model.Account, error)
	UnlockAccount(bic string, ban string, transferId model.TransferId) error

	database.PredictableCluster
}

type ClientCustomTx struct {
	shortId  uint64    // For logging
	clientId uuid.UUID // For locking
	cluster  CustomTxTransfer
	oracle   *database.Oracle
	payStats *PayStats
}

func (c *ClientCustomTx) Init(cluster CustomTxTransfer, oracle *database.Oracle, payStats *PayStats) {
	c.clientId = uuid.New()
	c.shortId = atomic.AddUint64(&nClients, 1)
	c.cluster = cluster
	c.oracle = oracle
	c.payStats = payStats

	llog.Tracef("[%v] Assigned client id %v", c.shortId, c.clientId)
}

func (c *ClientCustomTx) RegisterTransfer(t *model.Transfer) error {
	llog.Tracef("[%v] [%v] Registering %v", c.shortId, t.Id, t)
	// Register a new transfer
	err := c.cluster.InsertTransfer(t)
	if err != nil {
		// Should never happen, transfer id is globally unique
		llog.Fatalf("[%v] [%v] Failed to create: a duplicate transfer exists",
			c.shortId, t.Id)
		return merry.Prepend(err, "failed to insert transfer")
	}

	llog.Tracef("[%v] [%v] Setting client", c.shortId, t.Id)
	if err := c.SetTransferClient(t.Id); err != nil {
		return merry.Prepend(err, "failed to set transfer client")
	}

	return nil
}

// SetTransferClient
// Accept interfaces to allow nil client id
func (c *ClientCustomTx) SetTransferClient(transferId model.TransferId) error {
	return c.cluster.SetTransferClient(c.clientId, transferId)
}

// SetTransferState
// Accept interfaces to allow nil client id
func (c *ClientCustomTx) SetTransferState(t *model.Transfer, state string) error {
	llog.Tracef("[%v] [%v] Setting state %v", c.shortId, t.Id, state)

	if err := c.cluster.SetTransferState(state, t.Id, c.clientId); err != nil {
		return err
	}
	t.State = state
	return nil
}

// ClearTransferClient
// In case we failed for whatever reason try to clean up
// the transfer client, to allow speedy recovery
func (c *ClientCustomTx) ClearTransferClient(transferId model.TransferId) error {
	llog.Tracef("[%v] [%v] Clearing client", c.shortId, transferId)

	return c.cluster.ClearTransferClient(transferId, c.clientId)
}

func (c *ClientCustomTx) FetchAccountBalance(acc *model.Account) error {
	balance, pendingAmount, err := c.cluster.FetchBalance(acc.Bic, acc.Ban)
	if err != nil {
		return merry.Wrap(err)
	}

	acc.Balance = balance
	acc.PendingAmount = pendingAmount
	acc.Found = true

	llog.Warn("acc.Found")
	return nil
}

func (c *ClientCustomTx) UnlockAccount(transferId model.TransferId, account *model.Account) error {
	return c.cluster.UnlockAccount(account.Bic, account.Ban, transferId)
}

func (c *ClientCustomTx) LockAccounts(t *model.Transfer, wait bool) error {
	if t.State == "complete" {
		return nil
	}
	if t.State == "locked" {
		// The transfer is already locked - fetch balance to find out if the
		// account exists or not
		for i := 0; i < 2; i++ {
			if err := c.FetchAccountBalance(&t.Acs[i]); err != nil && err != cluster.ErrNoRows {
				return merry.Prepend(err, "failed to fetch account balance")
			}
		}
		llog.Tracef("[%v] [%v] Fetched locked %v", c.shortId, t.Id, t)
		return nil
	}

	llog.Tracef("[%v] [%v] Locking %v", c.shortId, t.Id, t)
	sleepDuration := time.Millisecond*time.Duration(rand.Intn(10)) + time.Millisecond
	maxSleepDuration, _ := time.ParseDuration("10s")

	// Upon failure to take lock on the second account, we should try to rollback
	// lock on the first to avoid deadlocks. We shouldn't, however, accidentally
	// rollback the lock if we haven't taken it - in this case lock0
	// and lock1 both may have been taken, and the transfer have progressed
	// to moving the funds, so rolling back the lock would break isolation.
	var previousAccount *model.Account

	i := 0
	for i < 2 {
		account := t.LockOrder[i]
		receivedAccount, err := c.cluster.LockAccount(t.Id, account.PendingAmount, account.Bic, account.Ban)
		if err != nil || t.Id.String() != receivedAccount.PendingTransfer.String() {
			if err != nil {
				// Remove the pending transfer from the previously
				// locked account, do not wait with locks.
				if i == 1 && previousAccount != nil {
					if unlockErr := c.UnlockAccount(t.Id, previousAccount); unlockErr != nil {
						return merry.Prepend(merry.WithCause(unlockErr, err), "failed to unlock locked accounts")
					}
				}
				// Check for transient errors, such as query timeout, and retry.
				// In case of a non-transient error, return it to the client.
				// No money changed its hands and the transfer can be recovered
				// later
				// No such account. We're not holding locks. CompleteTransfer() will delete
				// the transfer.
				if err == cluster.ErrNoRows {
					return merry.Prepend(c.SetTransferState(t, "locked"), "failed to set transfer state")
				} else if IsTransientError(err) {
					llog.Tracef("[%v] [%v] Retrying after error: %v", c.shortId, t.Id, err)
				} else {
					return merry.Prepend(err, "failed to execute lock accounts request")
				}
			}
			if t.Id != receivedAccount.PendingTransfer {
				// There is a non-empty pending transfer. Check if the
				// transfer we've conflicted with is orphaned and recover
				// it, before waiting
				var clientId *uuid.UUID
				clientId, err := c.cluster.FetchTransferClient(receivedAccount.PendingTransfer)
				if err != nil {
					if err != cluster.ErrNoRows {
						return merry.Prepend(err, "failed to fetch transfer client")
					}
					// Transfer not found, even though it's just aborted
					// our lock. It is OK, it might just got completed.
					llog.Tracef("[%v] [%v] Transfer %v which aborted our lock is now gone",
						c.shortId, t.Id, receivedAccount.PendingTransfer)
				} else if *clientId == model.NilUuid {
					// The transfer has no client working on it, recover it.
					llog.Tracef("[%v] [%v] Adding %v to the recovery queue",
						c.shortId, t.Id, receivedAccount.PendingTransfer)
					RecoverTransfer(receivedAccount.PendingTransfer)
				}
				atomic.AddUint64(&c.payStats.retries, 1)

				if !wait {
					return merry.New("failed to lock account: Wait aborted")
				}
			}
			// Restart locking
			i = 0

			time.Sleep(sleepDuration)
			runtime.Gosched()

			llog.Tracef("[%v] [%v] Restarting after sleeping %v",
				c.shortId, t.Id, sleepDuration)

			sleepDuration = sleepDuration * 2
			if sleepDuration > maxSleepDuration {
				sleepDuration = maxSleepDuration
			}
			t.Acs[0].Found = false
			t.Acs[1].Found = false
			previousAccount = nil
			// Reset client id in case it expired while we were sleeping
			if err := c.SetTransferClient(t.Id); err != nil {
				return merry.Prepend(err, "failed to set transfer client on reset")
			}
		} else {
			account.Found = true
			account.Balance = receivedAccount.Balance
			account.PendingAmount = receivedAccount.PendingAmount
			account.PendingTransfer = receivedAccount.PendingTransfer
			previousAccount = account
			i++
		}
	}
	// Move transfer to 'locked', to not attempt to transfer
	// 	the money twice during recovery
	return c.SetTransferState(t, "locked")
}

func (c *ClientCustomTx) CompleteTransfer(t *model.Transfer) error {
	if t.State != "locked" && t.State != "complete" {
		llog.Fatalf("[%v] [%v] Incorrect transfer state", c.shortId, t.Id)
	}

	acs := t.Acs
	if t.State == "locked" {
		if c.oracle != nil {
			c.oracle.BeginTransfer(t.Id, acs, t.Amount)
		}

		if acs[0].Found && acs[1].Found {
			// Calcualte the destination state
			llog.Tracef("[%v] [%v] Calculating balances for %v", c.shortId, t.Id, t)
			for i := 0; i < 2; i++ {
				acs[i].Balance.Add(acs[i].Balance, acs[i].PendingAmount)
			}

			if acs[0].Balance.Sign() >= 0 {
				llog.Tracef("[%v] [%v] Moving funds for %v", c.shortId, t.Id, t)
				for i := 0; i < 2; i++ {
					if err := c.cluster.UpdateBalance(acs[i].Balance, acs[i].Bic, acs[i].Ban, t.Id); err != nil {
						llog.Tracef("[%v] [%v] Failed to set account %v %v:%v to %v",
							c.shortId, t.Id, i, acs[i].Bic, acs[i].Ban, acs[i].Balance)
						return merry.Prepend(err, "failed to update balance")
					}
				}
			} else {
				llog.Tracef("[%v] [%v] Insufficient funds for %v", c.shortId, t.Id, t)
				atomic.AddUint64(&c.payStats.InsufficientFunds, 1)
			}
		} else {
			llog.Tracef("[%v] [%v] Account not found for %v", c.shortId, t.Id, t)

			atomic.AddUint64(&c.payStats.NoSuchAccount, 1)
		}
		if c.oracle != nil {
			c.oracle.CompleteTransfer(t.Id, acs, t.Amount)
		}
		if err := c.SetTransferState(t, "complete"); err != nil {
			return merry.Prepend(err, "failed to set transfer state")
		}
	}

	llog.Tracef("[%v] [%v] Unlocking %v", c.shortId, t.Id, t)

	for i := 0; i < 2; i++ {
		if err := c.UnlockAccount(t.Id, &acs[i]); err != nil && err != cluster.ErrNoRows {
			llog.Tracef("[%v] [%v] Failed to unlock account %v %v:%v: %v",
				c.shortId, t.Id, i, acs[i].Bic, acs[i].Ban, err)
			return merry.Prepend(err, "failed to unlock account")
		}
	}

	return c.DeleteTransfer(t.Id)
}

func (c *ClientCustomTx) DeleteTransfer(transferId model.TransferId) error {
	// Move transfer to "complete". Typically a transfer is kept
	// for a few years, we just delete it for simplicity.
	err := c.cluster.DeleteTransfer(transferId, c.clientId)
	if err != nil {
		if err == cluster.ErrNoRows {
			llog.Tracef("[%v] [%v] Transfer is already deleted", c.shortId, transferId)
			return nil
		}
		llog.Tracef("[%v] [%v] Failed to delete transfer: %v", c.shortId, transferId, err)
		return merry.Wrap(err)
	}

	return nil
}

func payWorkerCustomTx(
	settings config.DatabaseSettings,
	n_transfers int, zipfian bool, dbCluster CustomTxTransfer,
	oracle *database.Oracle, payStats *PayStats,
	wg *sync.WaitGroup) {

	defer wg.Done()

	var client ClientCustomTx
	var randSource fixed_random_source.FixedRandomSource
	client.Init(dbCluster, oracle, payStats)
	clusterSettings, err := dbCluster.FetchSettings()
	if err != nil {
		llog.Fatalf("Got a fatal error fetching cluster settings: %v", err)
	}

	randSource.Init(clusterSettings.Count, clusterSettings.Seed, settings.BanRangeMultiplier)

	for i := 0; i < n_transfers; {

		t := new(model.Transfer)
		t.InitRandomTransfer(&randSource, zipfian)

		cookie := statistics.StatsRequestStart()
		if err := client.MakeTransfer(t); err != nil {
			if err == cluster.ErrNoRows {
				llog.Tracef("[%v] [%v] Transfer not found", client.shortId, t.Id)
				i++
				statistics.StatsRequestEnd(cookie)
			} else if IsTransientError(err) {
				llog.Tracef("[%v] [%v] Transfer failed: %v", client.shortId, t.Id, err)
			} else {
				llog.Errorf("Got a fatal error %v, ending worker", err)
				return
			}
		} else {
			i++
			statistics.StatsRequestEnd(cookie)
		}
	}
}

//nolint:unparam
func payCustomTx(settings *config.DatabaseSettings, cluster CustomTxTransfer, oracle *database.Oracle) (*PayStats, error) {
	var wg sync.WaitGroup
	var payStats PayStats

	transfers_per_worker := settings.Count / settings.Workers
	remainder := settings.Count - transfers_per_worker*settings.Workers

	RecoveryStart(cluster, oracle, &payStats)

	clusterCustomTx, ok := cluster.(CustomTxTransfer)
	if !ok {
		llog.Fatalf("Custom transactions are not implemented for %s", cluster.GetClusterType())
	}

	for i := 0; i < settings.Workers; i++ {
		wg.Add(1)
		n_transfers := transfers_per_worker
		if i < remainder {
			n_transfers++
		}
		go payWorkerCustomTx(*settings, n_transfers, settings.ZIPFian, clusterCustomTx, oracle, &payStats, &wg)
	}

	wg.Wait()
	RecoveryStop()
	statistics.StatsReportSummary()
	if oracle != nil {
		oracle.FindBrokenAccounts(cluster)
	}

	return &payStats, nil
}

func (c *ClientCustomTx) MakeTransfer(t *model.Transfer) error {
	if err := c.RegisterTransfer(t); err != nil {
		return merry.Prepend(err, "failed to register transfer")
	}
	if err := c.LockAccounts(t, true); err != nil {
		return merry.Prepend(err, "failed to lock accounts")
	}
	if err := c.CompleteTransfer(t); err != nil {
		return merry.Prepend(err, "failed to complete transfer")
	}

	return nil
}

func (c *ClientCustomTx) RecoverTransfer(transferId model.TransferId) {
	cookie := statistics.StatsRequestStart()
	llog.Tracef("[%v] [%v] Recovering transfer", c.shortId, transferId)
	atomic.AddUint64(&c.payStats.recoveries, 1)
	if err := c.SetTransferClient(transferId); err != nil {
		if !merry.Is(err, cluster.ErrNoRows) {
			llog.Errorf("[%v] [%v] Failed to set client on transfer: %v",
				c.shortId, transferId, err)
		}
		return
	}

	// Ignore possible error, we will retry
	t, err := c.cluster.FetchTransfer(transferId)
	if err != nil {
		if err == cluster.ErrNoRows {
			llog.Errorf("[%v] [%v] Transfer not found when fetching for recovery",
				c.shortId, transferId)
			return
		} else {
			llog.Errorf("[%v] [%v] Failed to fetch transfer: %v",
				c.shortId, transferId, err)
			return
		}
	}

	t.InitAccounts()
	if err := c.LockAccounts(t, false); err != nil {
		llog.Errorf("[%v] [%v] Failed to lock accounts: %v",
			c.shortId, t.Id, err)
		if err := c.ClearTransferClient(t.Id); err != nil {
			llog.Errorf("[%v] [%v] Failed to clear transfer client: %v",
				c.shortId, t.Id, err)
		}
		return
	}
	if err := c.CompleteTransfer(t); err != nil {
		llog.Errorf("[%v] [%v] Failed to complete transfer during recovery: %v",
			c.shortId, t.Id, err)
	} else {
		statistics.StatsRequestEnd(cookie)
	}
}
