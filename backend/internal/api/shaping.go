package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/Kup1ng/Traffic-monitor/internal/shaper"
)

// handleShapingGet reports the current bandwidth-cap state: the persisted desired
// limit, whether qdiscs are installed, and whether this host can shape at all.
func (s *Server) handleShapingGet(w http.ResponseWriter, r *http.Request) {
	desired, err := s.eng.Store().GetBandwidthLimit()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read shaping state")
		return
	}
	writeJSON(w, http.StatusOK, s.shaper.Status(desired))
}

// handleShapingSet applies (mbps > 0) or clears (mbps <= 0) the bandwidth cap and
// persists the choice so it survives restarts. tc operations run under a bounded
// background context so a client disconnect can never abort a half-applied cap.
func (s *Server) handleShapingSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mbps int `json:"mbps"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Serialize the whole apply/clear-then-persist sequence so the kernel state
	// and the stored value can't diverge under concurrent requests.
	s.shapeMu.Lock()
	defer s.shapeMu.Unlock()

	st := s.eng.Store()
	ctx, cancel := shaper.BoundedContext()
	defer cancel()

	// Disable: persist first (so a failed write leaves the still-shaped kernel
	// consistent with the stored value), then tear the qdiscs down.
	if req.Mbps <= 0 {
		if err := st.SetBandwidthLimit(0); err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to save setting")
			return
		}
		_ = s.shaper.Clear(ctx)
		writeJSON(w, http.StatusOK, s.shaper.Status(0))
		return
	}

	if err := shaper.ValidateMbps(req.Mbps); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Apply first; only persist a limit we actually installed (Apply leaves the
	// interface unshaped on failure). On hosts that can't shape (demo / non-Linux)
	// we still record the desired value so it takes effect when deployed for real.
	if s.shaper.Supported() {
		if err := s.shaper.Apply(ctx, req.Mbps); err != nil {
			// Log the detailed tc/ip error server-side; don't leak it to the client.
			log.Printf("shaper: applying %d Mbps failed: %v", req.Mbps, err)
			msg := "failed to apply limit"
			// The usual cause is a service started without CAP_NET_ADMIN (e.g. a
			// binary-only update that didn't refresh the unit). Give an actionable hint.
			if strings.Contains(strings.ToLower(err.Error()), "not permitted") {
				msg = "failed to apply limit: the service lacks CAP_NET_ADMIN — run 'install.sh update' (or install) to refresh the systemd unit"
			}
			writeErr(w, http.StatusInternalServerError, msg)
			return
		}
	}
	if err := st.SetBandwidthLimit(req.Mbps); err != nil {
		log.Printf("shaper: cap applied (%d Mbps) but persisting it failed: %v — it will not survive a restart", req.Mbps, err)
		writeErr(w, http.StatusInternalServerError, "failed to save setting")
		return
	}
	writeJSON(w, http.StatusOK, s.shaper.Status(req.Mbps))
}
