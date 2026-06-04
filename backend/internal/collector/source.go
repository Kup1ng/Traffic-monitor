package collector

// CounterReader is the abstraction the traffic engine polls. Implementations:
//   - SysfsReader: real Linux interface counters (production)
//   - DemoReader:  synthetic, realistic-looking traffic (UI dev on any OS)
//   - FakeReader:  deterministic, test-controlled (unit tests)
//
// Read returns the cumulative rx/tx counters (monotonic since boot, reset to 0
// on reboot). BootID returns a token that changes on reboot, which lets the
// engine detect a counter reset exactly, independent of counter magnitude.
type CounterReader interface {
	Read() (rx, tx uint64, err error)
	BootID() string
	Iface() string
}

// Delta returns the bytes transferred between two consecutive raw counter
// readings. A counter that went backwards (cur < prev) means the counter was
// reset (reboot or NIC reset), so the new value itself is the delta — this never
// produces a negative result and never double-counts.
func Delta(cur, prev uint64) uint64 {
	if cur >= prev {
		return cur - prev
	}
	return cur
}

// SysfsReader reads real interface counters from /sys on Linux.
type SysfsReader struct {
	iface string
}

// NewSysfsReader returns a reader for the given interface.
func NewSysfsReader(iface string) *SysfsReader { return &SysfsReader{iface: iface} }

func (r *SysfsReader) Read() (uint64, uint64, error) { return ReadCounters(r.iface) }
func (r *SysfsReader) BootID() string                { return ReadBootID() }
func (r *SysfsReader) Iface() string                 { return r.iface }
