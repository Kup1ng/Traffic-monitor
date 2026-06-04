// Package engine is the traffic-accounting core. A single goroutine polls the
// interface counters, maintains an in-memory ring buffer for the live chart and
// running cumulative totals, and periodically flushes deltas to the store in a
// single atomic transaction.
//
// Correctness guarantees (never lose or double-count bytes):
//   - Reboots are detected exactly via boot_id (independent of counter
//     magnitude); the post-reboot counter value is the delta.
//   - A non-reboot counter that goes backwards (NIC reset) is also treated as a
//     reset (delta = current value) and triggers an immediate flush so the
//     durable anchor jumps to the post-reset value, shrinking the crash window.
//   - On startup, the delta since the last durable anchor is recovered, so a
//     crash (lost in-memory pending) and service downtime are both accounted for.
//   - Flushing subtracts exactly what was persisted from pending (never zeroes
//     it), so bytes that arrive during the flush transaction are not lost.
package engine

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Kup1ng/Traffic-monitor/internal/collector"
	"github.com/Kup1ng/Traffic-monitor/internal/config"
	"github.com/Kup1ng/Traffic-monitor/internal/store"
)

// fiveMinRetention is how long 5-minute rows are kept before pruning.
const fiveMinRetention = 24 * time.Hour

// Sample is one live throughput data point (bits per second).
type Sample struct {
	TS    int64   `json:"ts"`     // unix milliseconds
	RXbps float64 `json:"rx_bps"` // received bits/second
	TXbps float64 `json:"tx_bps"` // transmitted bits/second
}

// Engine owns the polling/accounting loop and exposes thread-safe accessors for
// the API layer.
type Engine struct {
	reader     collector.CounterReader
	store      *store.Store
	loc        *time.Location
	poll       time.Duration
	flushEvery time.Duration
	iface      string

	// Fields below marked (run) are touched only by the run goroutine (onTick,
	// flush, recover run sequentially), so they need no lock.
	lastRawRX, lastRawTX uint64 // (run) last raw counter seen
	pendRX, pendTX       uint64 // (run) bytes accumulated since last durable flush
	curBootID            string // (run)
	curHourKey           int64  // (run) UTC start-of-hour the pending belongs to
	curFiveKey           int64  // (run) UTC start-of-5-minute the pending belongs to

	mu             sync.Mutex // guards the API-visible fields below
	rxTotal        uint64     // cumulative received (DB total + pending)
	txTotal        uint64     // cumulative transmitted
	installUnix    int64
	lastUpdateUnix int64
	lastSample     Sample
	ring           *ring
}

// New constructs an Engine and runs startup recovery against the store. It must
// be called before Run.
func New(cfg *config.Config, reader collector.CounterReader, st *store.Store) (*Engine, error) {
	ringSize := int((5 * time.Minute) / cfg.PollInterval)
	if ringSize < 150 {
		ringSize = 150
	}
	e := &Engine{
		reader:     reader,
		store:      st,
		loc:        cfg.Location,
		poll:       cfg.PollInterval,
		flushEvery: cfg.FlushInterval,
		iface:      reader.Iface(),
		ring:       newRing(ringSize),
	}
	if err := e.recover(); err != nil {
		return nil, err
	}
	return e, nil
}

// recover loads persisted state and accounts for everything since the last
// durable anchor (crash-lost pending, downtime, or a reboot).
func (e *Engine) recover() error {
	rx, tx, err := e.reader.Read()
	if err != nil {
		return err
	}
	bid := e.reader.BootID()
	now := time.Now()
	hk, fk := hourKey(now), fiveKey(now)

	st, ok, err := e.store.LoadState()
	if err != nil {
		return err
	}

	if !ok {
		// First run ever: seed the anchor at the current counter so pre-install
		// traffic is not counted.
		if err := e.store.InitState(now.Unix(), e.iface, bid, rx, tx); err != nil {
			return err
		}
		e.rxTotal, e.txTotal = 0, 0
		e.installUnix = now.Unix()
	} else {
		if st.Iface != "" && st.Iface != e.iface {
			return &IfaceMismatchError{Configured: e.iface, Stored: st.Iface}
		}
		e.rxTotal, e.txTotal = st.RXTotal, st.TXTotal
		e.installUnix = st.InstallUnix

		var recRX, recTX uint64
		if bid != st.BootID {
			recRX, recTX = rx, tx // reboot: counters were zeroed; full value is the delta
		} else {
			recRX, recTX = collector.Delta(rx, st.LastRawRX), collector.Delta(tx, st.LastRawTX)
		}
		if recRX != 0 || recTX != 0 {
			if err := e.store.Flush(store.FlushArgs{
				DeltaRX: recRX, DeltaTX: recTX,
				RawRX: rx, RawTX: tx, BootID: bid, NowUnix: now.Unix(),
				HourKey: hk, FiveMinKey: fk,
				PruneBefore: fiveKey(now.Add(-fiveMinRetention)),
			}); err != nil {
				return err
			}
			e.rxTotal += recRX
			e.txTotal += recTX
		}
	}

	e.lastRawRX, e.lastRawTX = rx, tx
	e.pendRX, e.pendTX = 0, 0
	e.curBootID = bid
	e.curHourKey, e.curFiveKey = hk, fk
	e.lastUpdateUnix = now.Unix()
	e.lastSample = Sample{TS: now.UnixMilli()}
	return nil
}

// Run drives the poll and flush loop until ctx is cancelled, then performs a
// final flush and WAL checkpoint.
func (e *Engine) Run(ctx context.Context) error {
	pollT := time.NewTicker(e.poll)
	defer pollT.Stop()
	flushT := time.NewTicker(e.flushEvery)
	defer flushT.Stop()

	for {
		select {
		case <-ctx.Done():
			e.flush(time.Now(), e.curHourKey, e.curFiveKey)
			if err := e.store.Checkpoint(); err != nil {
				log.Printf("engine: checkpoint on shutdown: %v", err)
			}
			return nil
		case now := <-pollT.C:
			e.onTick(now)
		case now := <-flushT.C:
			e.flush(now, e.curHourKey, e.curFiveKey)
		}
	}
}

// onTick samples the counters once and updates live + pending state.
func (e *Engine) onTick(now time.Time) {
	rx, tx, err := e.reader.Read()
	if err != nil {
		// Transient read error (e.g. interface down): skip without touching the
		// anchor so no bytes are mis-attributed.
		return
	}
	bid := e.reader.BootID()

	reboot := bid != e.curBootID
	var dRX, dTX uint64
	if reboot {
		dRX, dTX = rx, tx
		e.curBootID = bid
	} else {
		dRX, dTX = collector.Delta(rx, e.lastRawRX), collector.Delta(tx, e.lastRawTX)
	}
	resetDown := rx < e.lastRawRX || tx < e.lastRawTX
	e.lastRawRX, e.lastRawTX = rx, tx

	secs := e.poll.Seconds()
	sample := Sample{TS: now.UnixMilli(), RXbps: float64(dRX) * 8 / secs, TXbps: float64(dTX) * 8 / secs}

	e.mu.Lock()
	e.pendRX += dRX
	e.pendTX += dTX
	e.rxTotal += dRX
	e.txTotal += dTX
	e.lastUpdateUnix = now.Unix()
	e.lastSample = sample
	e.ring.push(sample)
	e.mu.Unlock()

	hk, fk := hourKey(now), fiveKey(now)
	switch {
	case fk != e.curFiveKey:
		// Bucket rollover: persist accumulated pending into the bucket that just
		// ended, then switch to the new one. (A 5-minute boundary also covers any
		// hour boundary, since hours are multiples of 5 minutes.)
		e.flush(now, e.curHourKey, e.curFiveKey)
		e.curHourKey, e.curFiveKey = hk, fk
	case reboot || resetDown:
		// Counter reset without a bucket change: flush now to shrink the crash
		// window so the durable anchor reflects the post-reset counter.
		e.flush(now, hk, fk)
	}
}

// flush persists the current pending delta into the given bucket keys and
// advances the durable anchor, all in one transaction. On success it subtracts
// exactly what was persisted from pending (never zeroes it).
func (e *Engine) flush(now time.Time, hourK, fiveK int64) {
	e.mu.Lock()
	pr, pt := e.pendRX, e.pendTX
	e.mu.Unlock()

	rawRX, rawTX := e.lastRawRX, e.lastRawTX // run-goroutine only
	bid := e.curBootID

	if err := e.store.Flush(store.FlushArgs{
		DeltaRX: pr, DeltaTX: pt,
		RawRX: rawRX, RawTX: rawTX, BootID: bid, NowUnix: now.Unix(),
		HourKey: hourK, FiveMinKey: fiveK,
		PruneBefore: fiveKey(now.Add(-fiveMinRetention)),
	}); err != nil {
		log.Printf("engine: flush failed (will retry, no data lost): %v", err)
		return
	}

	// Subtract exactly what was persisted; bytes that arrived during the flush
	// remain in pending and are flushed next time.
	e.mu.Lock()
	e.pendRX -= pr
	e.pendTX -= pt
	e.mu.Unlock()
}

// --- API-facing accessors (thread-safe) ---

// Totals returns the cumulative totals (including not-yet-flushed pending) and
// install/last-update timestamps.
func (e *Engine) Totals() (rx, tx uint64, installUnix, lastUpdateUnix int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.rxTotal, e.txTotal, e.installUnix, e.lastUpdateUnix
}

// CurrentSpeed returns the most recent live sample.
func (e *Engine) CurrentSpeed() Sample {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastSample
}

// Pending returns the bytes accumulated since the last durable flush. Callers
// add this to bucket sums so totals reflect up-to-the-second activity.
func (e *Engine) Pending() (rx, tx uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pendRX, e.pendTX
}

// RecentSamples returns the live ring buffer in chronological order.
func (e *Engine) RecentSamples() []Sample {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ring.snapshot()
}

// Iface returns the monitored interface name.
func (e *Engine) Iface() string { return e.iface }

// Store exposes the underlying store for history queries.
func (e *Engine) Store() *store.Store { return e.store }

// Location returns the timezone used for day/month aggregation.
func (e *Engine) Location() *time.Location { return e.loc }

func hourKey(t time.Time) int64 { return t.Truncate(time.Hour).Unix() }
func fiveKey(t time.Time) int64 { return t.Truncate(5 * time.Minute).Unix() }
