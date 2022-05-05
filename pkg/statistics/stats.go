/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package statistics

import (
	"fmt"
	"time"

	llog "github.com/sirupsen/logrus"
	"github.com/spenczar/tdigest"
)

const (
	decimalToPercentCoefficient = 100
	percentile50                = 0.5
	percentile95                = 0.95
	percentile99                = 0.99
	percentile999               = 0.999
	channelBufferLength         = 1000
)

type Metrics struct {
	numberOfRequests int64
	cpuTime          time.Duration
	latencyMin       time.Duration
	latencyMax       time.Duration
	latencyAvg       time.Duration
	tDigest          *tdigest.TDigest
}

func (m *Metrics) Reset() {
	m.numberOfRequests = 0
	m.cpuTime = 0
	m.latencyMax = 0
	m.latencyMin = 0
	m.latencyAvg = 0
	m.tDigest = tdigest.New()
}

func (m *Metrics) Update(elapsed time.Duration) {
	m.numberOfRequests++
	m.cpuTime += elapsed

	if elapsed > m.latencyMax {
		m.latencyMax = elapsed
	}

	if m.latencyMin == 0 || m.latencyMin > elapsed {
		m.latencyMin = elapsed
	}

	m.tDigest.Add(elapsed.Seconds(), 1)
}

type stats struct {
	numTotal  int64
	startTime time.Time
	periodic  Metrics
	summary   Metrics
	queue     chan time.Duration
	done      chan bool
}

var s stats

type Cookie struct {
	time time.Time
}

func statsWorker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var more bool
loop:
	for {
		var elapsed time.Duration
		select {
		case <-ticker.C:
			if s.summary.numberOfRequests > 0 {
				var progress string
				if s.numTotal > 0 {
					progress = fmt.Sprintf("%5s%% done, RPS %d",
						fmt.Sprintf("%.2f", float64(s.summary.numberOfRequests)/float64(s.numTotal)*decimalToPercentCoefficient),
						s.periodic.numberOfRequests)
				} else {
					progress = fmt.Sprintf("Done %10d requests", s.summary.numberOfRequests)
				}

				llog.Infof("%s, Latency min/max/med: %.3fs/%.3fs/%.3fs",
					progress,
					s.periodic.latencyMin.Seconds(),
					s.periodic.latencyMax.Seconds(),
					s.periodic.tDigest.Quantile(percentile50),
				)
				s.periodic.Reset()
			}
		case elapsed, more = <-s.queue:
			if !more {
				break loop
			}
			s.periodic.Update(elapsed)
			s.summary.Update(elapsed)
		}
	}
	s.done <- true
}

func StatsSetTotal(n int) {
	s.numTotal = int64(n)
}

func StatsInit() {
	s.startTime = time.Now()
	s.periodic.Reset()
	s.summary.Reset()
	s.queue = make(chan time.Duration, channelBufferLength)
	s.done = make(chan bool, 1)

	go statsWorker()
}

func StatsRequestStart() Cookie {
	return Cookie{
		time: time.Now(),
	}
}

func StatsRequestEnd(c Cookie) {
	s.queue <- time.Since(c.time)
}

func StatsReportSummary() {
	// Stop background work
	close(s.queue)
	<-s.done

	if s.summary.numberOfRequests == 0 {
		return
	}

	wallClockTime := time.Since(s.startTime).Seconds()

	llog.Infof("Total time: %.3fs, %v t/sec",
		wallClockTime,
		int(float64(s.summary.numberOfRequests)/wallClockTime),
	)
	llog.Infof("Latency min/max/avg: %.3fs/%.3fs/%.3fs",
		s.summary.latencyMin.Seconds(),
		s.summary.latencyMax.Seconds(),
		s.summary.cpuTime.Seconds()/float64(s.summary.numberOfRequests),
	)
	llog.Infof("Latency 95/99/99.9%%: %.3fs/%.3fs/%.3fs",
		s.summary.tDigest.Quantile(percentile95),
		s.summary.tDigest.Quantile(percentile99),
		s.summary.tDigest.Quantile(percentile999),
	)
}
