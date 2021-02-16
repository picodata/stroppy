package main

import (
	"fmt"
	"time"

	llog "github.com/sirupsen/logrus"
	"github.com/spenczar/tdigest"
)

type Metrics struct {
	n_requests  int64
	cputime     time.Duration
	latency_min time.Duration
	latency_max time.Duration
	latency_avg time.Duration
	tdigest     *tdigest.TDigest
}

func (m *Metrics) Reset() {
	m.n_requests = 0
	m.cputime = 0
	m.latency_max = 0
	m.latency_min = 0
	m.latency_avg = 0
	m.tdigest = tdigest.New()
}

func (m *Metrics) Update(elapsed time.Duration) {
	m.n_requests++
	m.cputime += elapsed
	if elapsed > m.latency_max {
		m.latency_max = elapsed
	}
	if m.latency_min == 0 || m.latency_min > elapsed {
		m.latency_min = elapsed
	}
	m.tdigest.Add(elapsed.Seconds(), 1)
}

type stats struct {
	n_total   int64
	starttime time.Time
	periodic  Metrics
	summary   Metrics
	queue     chan time.Duration
	done      chan bool
}

var s stats

type cookie struct {
	time time.Time
}

func statsWorker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	more := true
loop:
	for {
		var elapsed time.Duration
		select {
		case <-ticker.C:
			if s.summary.n_requests > 0 {
				var progress string
				if s.n_total > 0 {
					progress = fmt.Sprintf("%5s%% done, RPS %d",
						fmt.Sprintf("%.2f", float64(s.summary.n_requests)/float64(s.n_total)*100),
						s.periodic.n_requests)
				} else {
					progress = fmt.Sprintf("Done %10d requests", s.summary.n_requests)
				}

				llog.Infof("%s, Latency min/max/med: %.3fs/%.3fs/%.3fs",
					progress,
					s.periodic.latency_min.Seconds(),
					s.periodic.latency_max.Seconds(),
					s.periodic.tdigest.Quantile(0.5),
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
	s.n_total = int64(n)
}

func StatsInit() {
	s.starttime = time.Now()
	s.periodic.Reset()
	s.summary.Reset()
	s.queue = make(chan time.Duration, 1000)
	s.done = make(chan bool, 1)
	go statsWorker()
}

func StatsRequestStart() cookie {
	return cookie{
		time: time.Now(),
	}
}

func StatsRequestEnd(c cookie) {
	s.queue <- time.Since(c.time)
}

func StatsReportSummary() {
	// Stop background work
	close(s.queue)
	<-s.done

	if s.summary.n_requests == 0 {
		return
	}

	wallclocktime := time.Since(s.starttime).Seconds()
	llog.Infof("Total time: %.3fs, %v t/sec",
		wallclocktime,
		int(float64(s.summary.n_requests)/wallclocktime),
	)
	llog.Infof("Latency min/max/avg: %.3fs/%.3fs/%.3fs",
		s.summary.latency_min.Seconds(),
		s.summary.latency_max.Seconds(),
		(s.summary.cputime.Seconds() / float64(s.summary.n_requests)),
	)
	llog.Infof("Latency 95/99/99.9%%: %.3fs/%.3fs/%.3fs",
		s.summary.tdigest.Quantile(0.95),
		s.summary.tdigest.Quantile(0.99),
		s.summary.tdigest.Quantile(0.999),
	)
}
