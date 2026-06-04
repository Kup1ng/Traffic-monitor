package store

import (
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStateLifecycle(t *testing.T) {
	s := openTemp(t)

	if _, ok, err := s.LoadState(); err != nil || ok {
		t.Fatalf("expected no state on fresh db, got ok=%v err=%v", ok, err)
	}

	if err := s.InitState(1000, "eth0", "boot-1", 500, 200); err != nil {
		t.Fatalf("init: %v", err)
	}
	st, ok, err := s.LoadState()
	if err != nil || !ok {
		t.Fatalf("load after init: ok=%v err=%v", ok, err)
	}
	if st.RXTotal != 0 || st.TXTotal != 0 || st.LastRawRX != 500 || st.LastRawTX != 200 ||
		st.BootID != "boot-1" || st.Iface != "eth0" || st.InstallUnix != 1000 {
		t.Fatalf("unexpected state after init: %+v", st)
	}
}

func TestFlushAccumulatesTotalsAndBuckets(t *testing.T) {
	s := openTemp(t)
	if err := s.InitState(0, "eth0", "boot-1", 0, 0); err != nil {
		t.Fatal(err)
	}

	const hour = 3_600
	const five = 300

	// Two flushes into the same buckets.
	if err := s.Flush(FlushArgs{DeltaRX: 100, DeltaTX: 40, RawRX: 100, RawTX: 40, BootID: "boot-1", NowUnix: 10, HourKey: hour, FiveMinKey: five}); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(FlushArgs{DeltaRX: 50, DeltaTX: 10, RawRX: 150, RawTX: 50, BootID: "boot-1", NowUnix: 20, HourKey: hour, FiveMinKey: five}); err != nil {
		t.Fatal(err)
	}

	st, _, err := s.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if st.RXTotal != 150 || st.TXTotal != 50 {
		t.Fatalf("totals = %d/%d, want 150/50", st.RXTotal, st.TXTotal)
	}
	if st.LastRawRX != 150 || st.LastRawTX != 50 {
		t.Fatalf("anchor = %d/%d, want 150/50", st.LastRawRX, st.LastRawTX)
	}

	hr, err := s.HourlyRange(0, hour+1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hr) != 1 || hr[0].TS != hour || hr[0].RX != 150 || hr[0].TX != 50 {
		t.Fatalf("hourly = %+v, want one bucket 150/50 at %d", hr, hour)
	}
	fm, err := s.FiveMinRange(0, five+1)
	if err != nil {
		t.Fatal(err)
	}
	if len(fm) != 1 || fm[0].RX != 150 || fm[0].TX != 50 {
		t.Fatalf("fivemin = %+v, want one bucket 150/50", fm)
	}
}

func TestFlushPrunesFiveMin(t *testing.T) {
	s := openTemp(t)
	if err := s.InitState(0, "eth0", "boot-1", 0, 0); err != nil {
		t.Fatal(err)
	}
	// Old 5-min bucket at t=0.
	if err := s.Flush(FlushArgs{DeltaRX: 10, DeltaTX: 5, RawRX: 10, RawTX: 5, BootID: "boot-1", NowUnix: 0, HourKey: 0, FiveMinKey: 0}); err != nil {
		t.Fatal(err)
	}
	// New bucket far later, pruning anything older than 24h.
	const day = 24 * 3600
	if err := s.Flush(FlushArgs{DeltaRX: 20, DeltaTX: 8, RawRX: 30, RawTX: 13, BootID: "boot-1", NowUnix: day + 600, HourKey: day, FiveMinKey: day + 600, PruneBefore: 600}); err != nil {
		t.Fatal(err)
	}
	fm, err := s.FiveMinRange(-1, day+3600)
	if err != nil {
		t.Fatal(err)
	}
	if len(fm) != 1 || fm[0].TS != day+600 {
		t.Fatalf("after prune fivemin = %+v, want only the new bucket", fm)
	}
	// Totals are unaffected by pruning.
	st, _, _ := s.LoadState()
	if st.RXTotal != 30 || st.TXTotal != 13 {
		t.Fatalf("totals = %d/%d, want 30/13", st.RXTotal, st.TXTotal)
	}
}

func TestSettingsAndSecrets(t *testing.T) {
	s := openTemp(t)

	if _, ok, _ := s.GetPasswordHash(); ok {
		t.Fatal("expected no password hash initially")
	}
	if err := s.SetPasswordHash("$2a$hash"); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := s.GetPasswordHash(); !ok || v != "$2a$hash" {
		t.Fatalf("password hash = %q ok=%v", v, ok)
	}
	if err := s.SetPasswordHash("$2a$new"); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := s.GetPasswordHash(); v != "$2a$new" {
		t.Fatalf("password hash after update = %q", v)
	}

	sec1, err := s.GetOrCreateSessionSecret()
	if err != nil || len(sec1) != 32 {
		t.Fatalf("secret1 len=%d err=%v", len(sec1), err)
	}
	sec2, err := s.GetOrCreateSessionSecret()
	if err != nil {
		t.Fatal(err)
	}
	if string(sec1) != string(sec2) {
		t.Fatal("session secret must be stable across calls")
	}
}
