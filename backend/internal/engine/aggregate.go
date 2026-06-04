package engine

import (
	"sort"
	"time"

	"github.com/Kup1ng/Traffic-monitor/internal/store"
)

// DailyBuckets groups hourly buckets into local-day buckets keyed by the unix
// timestamp of local midnight. Grouping happens at read time using loc, so the
// stored UTC hourly data is never affected by timezone or DST changes.
func DailyBuckets(hourly []store.Bucket, loc *time.Location) []store.Bucket {
	return groupBy(hourly, loc, func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	})
}

// MonthlyBuckets groups hourly buckets into local-month buckets keyed by the
// unix timestamp of the first day of the local month.
func MonthlyBuckets(hourly []store.Bucket, loc *time.Location) []store.Bucket {
	return groupBy(hourly, loc, func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
	})
}

// SumBuckets totals the rx/tx of a slice of buckets.
func SumBuckets(buckets []store.Bucket) (rx, tx uint64) {
	for _, b := range buckets {
		rx += b.RX
		tx += b.TX
	}
	return rx, tx
}

func groupBy(hourly []store.Bucket, loc *time.Location, startOf func(time.Time) time.Time) []store.Bucket {
	if loc == nil {
		loc = time.UTC
	}
	agg := make(map[int64]*store.Bucket)
	for _, b := range hourly {
		t := time.Unix(b.TS, 0).In(loc)
		key := startOf(t).Unix()
		cur, ok := agg[key]
		if !ok {
			cur = &store.Bucket{TS: key}
			agg[key] = cur
		}
		cur.RX += b.RX
		cur.TX += b.TX
	}
	out := make([]store.Bucket, 0, len(agg))
	for _, b := range agg {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS < out[j].TS })
	return out
}
