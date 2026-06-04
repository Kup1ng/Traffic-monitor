package engine

import (
	"testing"
	"time"

	"github.com/Kup1ng/Traffic-monitor/internal/store"
)

func TestDailyAndMonthlyAggregationUsesLocation(t *testing.T) {
	loc := time.FixedZone("UTC+2", 2*3600)

	hourUTC := func(y int, mo time.Month, d, h int, rx, tx uint64) store.Bucket {
		return store.Bucket{TS: time.Date(y, mo, d, h, 0, 0, 0, time.UTC).Unix(), RX: rx, TX: tx}
	}
	// In UTC+2: 21:00 UTC -> local 23:00 (Jan 1); 22:00 and 23:00 UTC -> local
	// Jan 2 00:00 and 01:00.
	hourly := []store.Bucket{
		hourUTC(2026, 1, 1, 21, 10, 1),
		hourUTC(2026, 1, 1, 22, 10, 1),
		hourUTC(2026, 1, 1, 23, 10, 1),
	}

	daily := DailyBuckets(hourly, loc)
	if len(daily) != 2 {
		t.Fatalf("daily buckets = %+v, want 2", daily)
	}
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc).Unix()
	jan2 := time.Date(2026, 1, 2, 0, 0, 0, 0, loc).Unix()
	if daily[0].TS != jan1 || daily[0].RX != 10 {
		t.Fatalf("daily[0] = %+v, want ts=%d rx=10", daily[0], jan1)
	}
	if daily[1].TS != jan2 || daily[1].RX != 20 {
		t.Fatalf("daily[1] = %+v, want ts=%d rx=20", daily[1], jan2)
	}

	monthly := MonthlyBuckets(hourly, loc)
	if len(monthly) != 1 {
		t.Fatalf("monthly buckets = %+v, want 1", monthly)
	}
	monthStart := time.Date(2026, 1, 1, 0, 0, 0, 0, loc).Unix()
	if monthly[0].TS != monthStart || monthly[0].RX != 30 || monthly[0].TX != 3 {
		t.Fatalf("monthly[0] = %+v, want ts=%d rx=30 tx=3", monthly[0], monthStart)
	}

	if rx, tx := SumBuckets(hourly); rx != 30 || tx != 3 {
		t.Fatalf("sum = %d/%d, want 30/3", rx, tx)
	}
}
