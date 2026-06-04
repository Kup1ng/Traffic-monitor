package engine

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Kup1ng/Traffic-monitor/internal/collector"
	"github.com/Kup1ng/Traffic-monitor/internal/config"
	"github.com/Kup1ng/Traffic-monitor/internal/store"
)

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func buildEngine(t *testing.T, st *store.Store, r collector.CounterReader) *Engine {
	t.Helper()
	cfg := &config.Config{PollInterval: 2 * time.Second, FlushInterval: time.Minute, Location: time.UTC}
	e, err := New(cfg, r, st)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	return e
}

func at(sec int) time.Time { return base.Add(time.Duration(sec) * time.Second) }

func wantTotals(t *testing.T, e *Engine, rx, tx uint64) {
	t.Helper()
	grx, gtx, _, _ := e.Totals()
	if grx != rx || gtx != tx {
		t.Fatalf("totals = %d/%d, want %d/%d", grx, gtx, rx, tx)
	}
}

// First run: anchor is seeded at the current counter (pre-install traffic is not
// counted), then deltas accumulate exactly and persist.
func TestFirstRunAccumulation(t *testing.T) {
	st := openStore(t)
	r := collector.NewFakeReader("eth0", "boot-1")
	r.Set(1000, 500) // counters already at 1000/500 since boot when we install
	e := buildEngine(t, st, r)

	wantTotals(t, e, 0, 0)

	r.Add(100, 40)
	e.onTick(at(2))
	r.Add(200, 60)
	e.onTick(at(4))
	wantTotals(t, e, 300, 100)

	if s := e.CurrentSpeed(); s.RXbps != 200*8/2 || s.TXbps != 60*8/2 {
		t.Fatalf("speed = %+v, want rx=800 tx=240", s)
	}

	e.flush(at(4), e.curHourKey, e.curFiveKey)
	stState, _, _ := st.LoadState()
	if stState.RXTotal != 300 || stState.TXTotal != 100 {
		t.Fatalf("db totals = %d/%d, want 300/100", stState.RXTotal, stState.TXTotal)
	}
	if stState.LastRawRX != 1300 || stState.LastRawTX != 600 {
		t.Fatalf("db anchor = %d/%d, want 1300/600", stState.LastRawRX, stState.LastRawTX)
	}
}

// A NIC counter reset (counter goes backwards, same boot) is treated as a reset:
// the new value is the delta, with no negative or spurious huge jump.
func TestNicResetMidRun(t *testing.T) {
	st := openStore(t)
	r := collector.NewFakeReader("eth0", "boot-1")
	r.Set(1000, 500)
	e := buildEngine(t, st, r)

	r.Add(100, 40)
	e.onTick(at(2)) // totals 100/40, anchor 1100/540
	r.Set(30, 10)   // counter reset to a smaller value
	e.onTick(at(4)) // delta = 30/10
	wantTotals(t, e, 130, 50)
}

// A reboot (boot_id changes, counters zeroed) continues the cumulative totals.
func TestRebootContinues(t *testing.T) {
	st := openStore(t)
	r := collector.NewFakeReader("eth0", "boot-1")
	e := buildEngine(t, st, r)

	r.Add(1000, 400)
	e.onTick(at(2)) // totals 1000/400
	r.Reboot("boot-2")
	r.Add(50, 20)
	e.onTick(at(4)) // reboot: delta = 50/20
	wantTotals(t, e, 1050, 420)
	if e.curBootID != "boot-2" {
		t.Fatalf("boot id not updated: %q", e.curBootID)
	}
}

// A crash that loses in-memory pending, plus traffic during downtime, are both
// recovered on the next startup from the durable anchor.
func TestCrashRecoversPendingAndDowntime(t *testing.T) {
	st := openStore(t)
	rA := collector.NewFakeReader("eth0", "boot-1")
	rA.Set(1000, 500)
	eA := buildEngine(t, st, rA)

	rA.Add(100, 40)
	eA.onTick(at(2))
	eA.flush(at(2), eA.curHourKey, eA.curFiveKey) // DB total 100/40, anchor 1100/540

	rA.Add(70, 30)
	eA.onTick(at(4)) // pending 70/30 in memory, NOT flushed

	// "crash": eA is abandoned without flushing. More traffic during downtime:
	rA.Add(200, 80) // counter now 1370/650

	// New process starts on the same DB with the current counter, same boot.
	rB := collector.NewFakeReader("eth0", "boot-1")
	rB.Set(1370, 650)
	eB := buildEngine(t, st, rB)

	// 100 flushed + recovery(1370-1100=270 / 650-540=110) = 370/150 == all traffic.
	wantTotals(t, eB, 370, 150)
}

// D2 fix: after a reboot during downtime, recovery must use the full post-reboot
// counter even when it already exceeds the old anchor (boot_id disambiguates).
func TestRebootDuringDowntimeBootIDFix(t *testing.T) {
	st := openStore(t)
	rA := collector.NewFakeReader("eth0", "boot-1")
	eA := buildEngine(t, st, rA)

	rA.Add(1000, 400)
	eA.onTick(at(2))
	eA.flush(at(2), eA.curHourKey, eA.curFiveKey) // DB total 1000/400, anchor 1000/400

	// Reboot during downtime; heavy traffic since the new boot exceeds the old
	// anchor (3000 > 1000). A magnitude-only check would lose the 0..1000 part.
	rB := collector.NewFakeReader("eth0", "boot-2")
	rB.Set(3000, 1200)
	eB := buildEngine(t, st, rB)

	wantTotals(t, eB, 4000, 1600)
}

// Totals stay exact well past the 2^53 float-precision limit (uint64 end to
// end, including the SQLite round-trip).
func TestMultiTerabyteExact(t *testing.T) {
	st := openStore(t)
	r := collector.NewFakeReader("eth0", "boot-1")
	e := buildEngine(t, st, r)

	const chunk = uint64(1_000_000_000_000_000) // 1 PB per tick (math stress)
	var want uint64
	for i := 0; i < 20; i++ {
		r.Add(chunk, chunk/2)
		e.onTick(at(2 * (i + 1)))
		want += chunk
	}
	if want <= 1<<53 {
		t.Fatalf("test does not exceed 2^53; want=%d", want)
	}
	wantTotals(t, e, want, want/2)

	e.flush(at(100), e.curHourKey, e.curFiveKey)
	stState, _, _ := st.LoadState()
	if stState.RXTotal != want || stState.TXTotal != want/2 {
		t.Fatalf("db totals = %d/%d, want %d/%d", stState.RXTotal, stState.TXTotal, want, want/2)
	}
}

// Crossing a 5-minute boundary flushes the accumulated pending into the bucket
// that just ended, and totals remain exact.
func TestBucketRolloverAttribution(t *testing.T) {
	st := openStore(t)
	r := collector.NewFakeReader("eth0", "boot-1")
	e := buildEngine(t, st, r)
	// Align the engine's current-bucket keys with the synthetic test clock
	// (recover seeds them from real time.Now(); production ticks use the same
	// clock as the seed, so this only matters for time-warped tests).
	e.curHourKey = hourKey(at(0))
	e.curFiveKey = fiveKey(at(0))

	// Two ticks inside the 00:00 bucket.
	r.Add(100, 10)
	e.onTick(at(2))
	r.Add(100, 10)
	e.onTick(at(4))
	// Tick inside the 00:05 bucket -> rollover flushes 00:00 (incl. this delta).
	r.Add(50, 5)
	e.onTick(at(5*60 + 2))
	// Another tick, then an explicit periodic flush into 00:05.
	r.Add(100, 10)
	e.onTick(at(5*60 + 4))
	e.flush(at(5*60+4), e.curHourKey, e.curFiveKey)

	fm, err := st.FiveMinRange(base.Unix()-1, base.Add(time.Hour).Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(fm) != 2 {
		t.Fatalf("expected 2 five-minute buckets, got %+v", fm)
	}
	if fm[0].TS != base.Unix() || fm[0].RX != 250 || fm[0].TX != 25 {
		t.Fatalf("bucket A = %+v, want ts=%d rx=250 tx=25", fm[0], base.Unix())
	}
	if fm[1].TS != base.Unix()+300 || fm[1].RX != 100 || fm[1].TX != 10 {
		t.Fatalf("bucket B = %+v, want ts=%d rx=100 tx=10", fm[1], base.Unix()+300)
	}

	hr, _ := st.HourlyRange(base.Unix()-1, base.Add(time.Hour).Unix())
	if len(hr) != 1 || hr[0].RX != 350 || hr[0].TX != 35 {
		t.Fatalf("hourly = %+v, want one bucket 350/35", hr)
	}
	wantTotals(t, e, 350, 35)
}

// New refuses to merge counters from a different interface than the one stored.
func TestInterfaceMismatch(t *testing.T) {
	st := openStore(t)
	r1 := collector.NewFakeReader("eth0", "boot-1")
	buildEngine(t, st, r1) // seeds state with iface eth0

	r2 := collector.NewFakeReader("wlan0", "boot-1")
	cfg := &config.Config{PollInterval: 2 * time.Second, FlushInterval: time.Minute, Location: time.UTC}
	_, err := New(cfg, r2, st)
	var mism *IfaceMismatchError
	if !errors.As(err, &mism) {
		t.Fatalf("expected IfaceMismatchError, got %v", err)
	}
}

// A boot_id token appearing (""-> non-empty) without an actual reboot must not
// be misread as a reboot — otherwise the whole counter would be re-counted.
func TestEmptyToNonEmptyBootIDNotReboot(t *testing.T) {
	st := openStore(t)
	r := collector.NewFakeReader("eth0", "") // boot_id initially unreadable
	r.Set(1000, 500)
	e := buildEngine(t, st, r)

	r.Add(100, 40)
	e.onTick(at(2)) // totals 100/40, anchor 1100/540, curBootID ""

	// boot_id becomes readable; counters keep climbing (no real reboot).
	r.SetBootID("boot-xyz")
	r.Add(50, 20)
	e.onTick(at(4)) // must be a normal delta of 50/20, NOT a full re-count
	wantTotals(t, e, 150, 60)
	if e.curBootID != "boot-xyz" {
		t.Fatalf("boot id not adopted: %q", e.curBootID)
	}
}

// Exercises the live runtime path (Run loop ticking + flushing) concurrently
// with the API-facing accessors and a traffic generator. Run with -race to
// catch data races on the engine's shared fields.
func TestConcurrentAccessRace(t *testing.T) {
	st := openStore(t)
	r := collector.NewFakeReader("eth0", "boot-1")
	cfg := &config.Config{PollInterval: 5 * time.Millisecond, FlushInterval: 15 * time.Millisecond, Location: time.UTC}
	e, err := New(cfg, r, st)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = e.Run(ctx); close(done) }()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Traffic generator (FakeReader is mutex-guarded).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				r.Add(1000, 400)
			}
		}
	}()

	// Concurrent readers hammering every accessor.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _, _, _ = e.Totals()
					_ = e.CurrentSpeed()
					_ = e.RecentSamples()
					_, _ = e.Pending()
				}
			}
		}()
	}

	time.Sleep(150 * time.Millisecond)
	close(stop)
	wg.Wait()
	cancel()
	<-done

	if rx, _, _, _ := e.Totals(); rx == 0 {
		t.Fatal("expected some traffic to be accounted")
	}
}
