package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// handleStream streams live throughput samples as Server-Sent Events, pushing
// the current speed every poll interval. The browser consumes it with
// EventSource (cookies are sent automatically for same-origin requests).
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // disable proxy buffering (e.g. nginx)

	send := func() bool {
		b, err := json.Marshal(s.eng.CurrentSpeed())
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// Send an immediate sample so the chart updates without waiting a full tick.
	if !send() {
		return
	}

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}
