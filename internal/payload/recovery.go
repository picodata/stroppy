/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package payload

import (
	"sync"

	"gitlab.com/picodata/stroppy/pkg/database"

	"gitlab.com/picodata/stroppy/internal/model"

	llog "github.com/sirupsen/logrus"
)

func recoveryWorker(cluster CustomTxTransfer, oracle *database.Oracle, payStats *PayStats,
	wg *sync.WaitGroup) {
	defer wg.Done()

	c := ClientCustomTx{}
	c.Init(cluster, oracle, payStats)

loop:
	for {
		transferId, more := <-q.queue
		if !more {
			break loop
		}
		c.RecoverTransfer(transferId)
	}
}

type RecoveryQueue struct {
	queue    chan model.TransferId
	wg       sync.WaitGroup
	cluster  CustomTxTransfer
	oracle   *database.Oracle
	payStats *PayStats
}

func (q *RecoveryQueue) Init(cluster CustomTxTransfer, oracle *database.Oracle, payStats *PayStats) {
	q.cluster = cluster
	q.oracle = oracle
	q.payStats = payStats

	// Recovery is recursive, create the channels first
	// what kind of magic number 4096000 is?
	const queueCapacity = 4096000
	q.queue = make(chan model.TransferId, queueCapacity)
}

func (q *RecoveryQueue) StartRecoveryWorker() {
	q.wg.Add(1)
	go recoveryWorker(q.cluster, q.oracle, q.payStats, &q.wg)
}

func (q *RecoveryQueue) Stop() {
	close(q.queue)
	q.wg.Wait()
}

var q RecoveryQueue

func RecoverTransfer(transferId model.TransferId) {
	q.queue <- transferId
}

func Recover() {
	c := ClientCustomTx{}
	c.Init(q.cluster, q.oracle, q.payStats)

	llog.Infof("Fetching dead transfers")

	transferIds, err := q.cluster.FetchDeadTransfers()
	if err != nil {
		llog.Errorf("Failed to fetch dead transfers: %v", err)
	}
	if len(transferIds) != 0 {
		llog.Infof("Found %v outstanding transfers, recovering...", len(transferIds))
		for _, transferId := range transferIds {
			c.RecoverTransfer(transferId)
		}
	}
}

func RecoveryStart(cluster CustomTxTransfer, oracle *database.Oracle, payStats *PayStats) {
	q.Init(cluster, oracle, payStats)

	// Start background fiber working on the queue to
	// make sure we purge it even during the initial recovery
	for i := 0; i < 8; i++ {
		q.StartRecoveryWorker()
	}

	Recover()
}

func RecoveryStop() {
	Recover()
	q.Stop()
}
