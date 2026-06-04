package collector

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// DemoReader synthesizes realistic, wavy traffic counters so the dashboard can
// be developed and demonstrated on any OS (including Windows/macOS) without a
// real interface. Counters advance by a sinusoidal-plus-noise byte rate scaled
// by the elapsed time between reads, so the engine's delta/speed math behaves
// exactly as it would against a real NIC.
type DemoReader struct {
	mu       sync.Mutex
	rx, tx   uint64
	bootID   string
	iface    string
	start    time.Time
	lastRead time.Time
	rnd      *rand.Rand
}

// NewDemoReader creates a synthetic reader. If iface is empty it is labelled
// "demo0".
func NewDemoReader(iface string) *DemoReader {
	if iface == "" {
		iface = "demo0"
	}
	now := time.Now()
	return &DemoReader{
		iface:  iface,
		bootID: "demo-boot",
		start:  now,
		rnd:    rand.New(rand.NewSource(now.UnixNano())),
	}
}

func (d *DemoReader) Read() (uint64, uint64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	var dt float64
	if !d.lastRead.IsZero() {
		dt = now.Sub(d.lastRead).Seconds()
	}
	d.lastRead = now

	t := now.Sub(d.start).Seconds()
	// Download-heavy profile: ~0.15–2.5 MB/s down, ~0.04–0.5 MB/s up.
	dlRate := 1_500_000.0*(1+math.Sin(t/15.0))/2 + d.rnd.Float64()*800_000 + 150_000
	ulRate := 250_000.0*(1+math.Sin(t/9.0+1.0))/2 + d.rnd.Float64()*120_000 + 40_000

	d.rx += uint64(dlRate * dt)
	d.tx += uint64(ulRate * dt)
	return d.rx, d.tx, nil
}

func (d *DemoReader) BootID() string { return d.bootID }
func (d *DemoReader) Iface() string  { return d.iface }
