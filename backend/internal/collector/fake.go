package collector

import "sync"

// FakeReader is a deterministic, test-controlled CounterReader. Tests set the
// raw counters and boot_id explicitly to simulate steady traffic, counter
// resets, and reboots. It is also handy for integration tests of the API.
type FakeReader struct {
	mu     sync.Mutex
	rx, tx uint64
	bootID string
	iface  string
}

// NewFakeReader creates a fake reader with the given interface label and initial
// boot_id.
func NewFakeReader(iface, bootID string) *FakeReader {
	return &FakeReader{iface: iface, bootID: bootID}
}

func (f *FakeReader) Read() (uint64, uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rx, f.tx, nil
}

func (f *FakeReader) BootID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bootID
}

func (f *FakeReader) Iface() string { return f.iface }

// Set replaces the raw counters (e.g. to simulate a NIC counter reset).
func (f *FakeReader) Set(rx, tx uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rx, f.tx = rx, tx
}

// Add advances the raw counters by the given amounts (normal traffic).
func (f *FakeReader) Add(rx, tx uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rx += rx
	f.tx += tx
}

// Reboot simulates a reboot: counters reset to zero and the boot_id changes.
func (f *FakeReader) Reboot(newBootID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rx, f.tx = 0, 0
	f.bootID = newBootID
}

// SetBootID changes the boot_id token WITHOUT touching the counters (e.g. to
// simulate boot_id becoming readable mid-run without an actual reboot).
func (f *FakeReader) SetBootID(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bootID = id
}
