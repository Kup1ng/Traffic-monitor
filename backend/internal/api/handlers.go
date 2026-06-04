package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Kup1ng/Traffic-monitor/internal/collector"
	"github.com/Kup1ng/Traffic-monitor/internal/engine"
	"github.com/Kup1ng/Traffic-monitor/internal/store"
)

// jsonBucket is a time-bucket with byte counts serialized as decimal strings so
// values beyond 2^53 stay exact in JavaScript (parsed with BigInt on the client).
type jsonBucket struct {
	TS int64  `json:"ts"`
	RX string `json:"rx"`
	TX string `json:"tx"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func u64(v uint64) string { return strconv.FormatUint(v, 10) }

func toJSONBuckets(bs []store.Bucket) []jsonBucket {
	out := make([]jsonBucket, len(bs))
	for i, b := range bs {
		out[i] = jsonBucket{TS: b.TS, RX: u64(b.RX), TX: u64(b.TX)}
	}
	return out
}

// mergePending folds the not-yet-flushed pending bytes into the current bucket
// so charts and sums reflect up-to-the-second activity.
func mergePending(buckets []store.Bucket, curKey int64, pr, pt uint64) []store.Bucket {
	if pr == 0 && pt == 0 {
		return buckets
	}
	for i := range buckets {
		if buckets[i].TS == curKey {
			buckets[i].RX += pr
			buckets[i].TX += pt
			return buckets
		}
	}
	return append(buckets, store.Bucket{TS: curKey, RX: pr, TX: pt})
}

// --- auth/session ---

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.auth.AllowLogin(r) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, please wait a moment")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ok, err := s.auth.CheckPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok {
		s.auth.NoteLoginFailure(r)
		writeErr(w, http.StatusUnauthorized, "invalid password")
		return
	}
	s.auth.NoteLoginSuccess(r)
	s.auth.SetSession(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.ClearSession(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"authenticated":       s.auth.Authenticated(r),
		"password_configured": s.auth.PasswordConfigured(),
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.version})
}

// --- totals / live ---

func (s *Server) handleTotals(w http.ResponseWriter, r *http.Request) {
	rx, tx, install, last := s.eng.Totals()
	writeJSON(w, http.StatusOK, map[string]any{
		"rx_total":         u64(rx),
		"tx_total":         u64(tx),
		"total":            u64(rx + tx),
		"install_unix":     install,
		"last_update_unix": last,
		"interface":        s.eng.Iface(),
	})
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.CurrentSpeed())
}

func (s *Server) handleRecent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.RecentSamples())
}

// --- history ---

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rng := q.Get("range")
	now := time.Now()
	loc := s.eng.Location()
	st := s.eng.Store()

	var buckets []store.Bucket
	var err error
	var curKey int64

	switch rng {
	case "5min":
		n := parseCount(q.Get("count"), 288, 2016) // default 24h, max 7d
		buckets, err = st.FiveMinRange(now.Add(-time.Duration(n)*5*time.Minute).Unix(), now.Unix()+1)
		curKey = now.Truncate(5 * time.Minute).Unix()
	case "hour":
		n := parseCount(q.Get("count"), 48, 24*90)
		buckets, err = st.HourlyRange(now.Add(-time.Duration(n)*time.Hour).Unix(), now.Unix()+3600)
		curKey = now.Truncate(time.Hour).Unix()
	case "day":
		n := parseCount(q.Get("count"), 30, 366*5)
		var hourly []store.Bucket
		hourly, err = st.HourlyRange(now.AddDate(0, 0, -n).Unix(), now.Unix()+3600)
		if err == nil {
			buckets = engine.DailyBuckets(hourly, loc)
		}
		y, m, d := now.In(loc).Date()
		curKey = time.Date(y, m, d, 0, 0, 0, 0, loc).Unix()
	case "month":
		n := parseCount(q.Get("count"), 12, 120)
		var hourly []store.Bucket
		hourly, err = st.HourlyRange(now.AddDate(0, -n, 0).Unix(), now.Unix()+3600)
		if err == nil {
			buckets = engine.MonthlyBuckets(hourly, loc)
		}
		y, m, _ := now.In(loc).Date()
		curKey = time.Date(y, m, 1, 0, 0, 0, 0, loc).Unix()
	default:
		writeErr(w, http.StatusBadRequest, "range must be one of: 5min, hour, day, month")
		return
	}

	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read history")
		return
	}

	pr, pt := s.eng.Pending()
	buckets = mergePending(buckets, curKey, pr, pt)

	writeJSON(w, http.StatusOK, map[string]any{
		"range":   rng,
		"buckets": toJSONBuckets(buckets),
	})
}

// --- interface / summary ---

func (s *Server) handleInterface(w http.ResponseWriter, r *http.Request) {
	info := collector.GetInterfaceInfo(s.eng.Iface())
	speed := s.eng.CurrentSpeed()
	writeJSON(w, http.StatusOK, map[string]any{
		"name":       info.Name,
		"mac":        info.MAC,
		"mtu":        info.MTU,
		"speed_mbps": info.SpeedMbps,
		"operstate":  info.OperState,
		"addrs":      info.Addrs,
		"current_rx": speed.RXbps,
		"current_tx": speed.TXbps,
		"demo":       s.cfg.Demo,
		"poll_ms":    s.cfg.PollInterval.Milliseconds(),
	})
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	loc := s.eng.Location()
	st := s.eng.Store()
	rxTotal, txTotal, install, last := s.eng.Totals()
	pr, pt := s.eng.Pending()

	// Period sums come from flushed buckets plus the current pending, so they are
	// non-zero immediately and consistent with the all-time total.
	sum := func(bs []store.Bucket, _ error) (uint64, uint64) {
		rx, tx := engine.SumBuckets(bs)
		return rx + pr, tx + pt
	}

	last24rx, last24tx := sum(st.FiveMinRange(now.Add(-24*time.Hour).Unix(), now.Unix()+1))

	y, m, d := now.In(loc).Date()
	todayStart := time.Date(y, m, d, 0, 0, 0, 0, loc)
	todayRx, todayTx := sum(st.HourlyRange(todayStart.Unix(), now.Unix()+3600))

	monthStart := time.Date(y, m, 1, 0, 0, 0, 0, loc)
	monthRx, monthTx := sum(st.HourlyRange(monthStart.Unix(), now.Unix()+3600))

	writeJSON(w, http.StatusOK, map[string]any{
		"all_time":         period(u64(rxTotal), u64(txTotal)), // already includes pending
		"last_24h":         period(u64(last24rx), u64(last24tx)),
		"today":            period(u64(todayRx), u64(todayTx)),
		"this_month":       period(u64(monthRx), u64(monthTx)),
		"install_unix":     install,
		"last_update_unix": last,
	})
}

func period(rx, tx string) map[string]string {
	return map[string]string{"rx": rx, "tx": tx}
}

func parseCount(v string, def, max int) int {
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}
