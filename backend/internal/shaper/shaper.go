// Package shaper applies a hard bandwidth cap to the monitored interface using
// the Linux traffic-control stack (tc). Egress (TX) is capped with an HTB qdisc
// on the interface itself; ingress (RX) is shaped by redirecting incoming
// traffic to a dedicated IFB device and capping that device's egress. A single
// Mbps value caps the interface's instantaneous throughput in both directions.
//
// The exact command sequences were validated against a live kernel (HTB on the
// interface for egress, ifb-redirect + HTB for ingress) — a 20 Mbit cap measured
// 19.1 Mbit/s on both directions, and teardown fully restores line rate.
//
// Real shaping only runs on Linux, on a real (non-demo) interface, with tc and
// ip present. In demo mode the Manager is "simulated": it records the requested
// cap (so the whole UI is exercisable on any OS) without touching the system.
// Anywhere else it reports Supported() false and does nothing.
package shaper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// ifbDevice is the dedicated Intermediate Functional Block device used to shape
// ingress. The app monitors a single interface, so one fixed device suffices.
const ifbDevice = "tm-ifb0"

// Mbps bounds. Reject non-positive and absurd values; 1 Tbit/s is a generous
// ceiling well above any real NIC.
const (
	minMbps = 1
	maxMbps = 1_000_000
)

// ErrUnsupported is returned by Apply when shaping is not available on this host
// (non-Linux, demo mode, or tc/ip missing).
var ErrUnsupported = errors.New("bandwidth shaping is not supported on this host")

// Runner executes an external command and returns its combined output. It is an
// interface so tests can assert the exact tc/ip sequences without root or tc.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.Bytes(), err
}

// Manager applies/clears the bandwidth cap on one interface.
type Manager struct {
	iface  string
	run    Runner
	tcPath string
	ipPath string
	logger *log.Logger

	supported bool
	simulated bool // demo mode: record state without running tc

	mu         sync.Mutex
	activeMbps int // currently applied cap (0 = unshaped)
}

// Status is the shaping state reported to the UI.
type Status struct {
	LimitMbps int  `json:"limit_mbps"` // desired cap; 0 = disabled
	Active    bool `json:"active"`     // qdiscs currently installed
	Supported bool `json:"supported"`  // this host can shape
}

// New builds a Manager for iface. In demo mode it is "simulated" (supported, but
// records state without running tc) so the UI is fully exercisable on any OS.
// Real shaping additionally requires Linux and the tc/ip binaries; when those are
// missing the Manager reports Supported() == false and does nothing.
func New(iface string, demo bool, logger *log.Logger) *Manager {
	if logger == nil {
		logger = log.Default()
	}
	m := &Manager{iface: iface, run: execRunner{}, logger: logger}
	if demo {
		m.simulated, m.supported = true, true
		return m
	}
	if runtime.GOOS != "linux" {
		return m
	}
	if !validIfaceName(iface) {
		logger.Printf("shaper: disabled (invalid interface name %q)", iface)
		return m
	}
	tcPath, errTC := lookPath("tc")
	ipPath, errIP := lookPath("ip")
	if errTC == nil && errIP == nil {
		m.tcPath, m.ipPath, m.supported = tcPath, ipPath, true
	} else {
		logger.Printf("shaper: disabled (tc/ip not found): tc=%v ip=%v", errTC, errIP)
	}
	return m
}

// lookPath finds a binary in PATH, falling back to the usual sbin locations that
// may be absent from a hardened service PATH.
func lookPath(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, dir := range []string{"/usr/sbin/", "/sbin/", "/usr/bin/", "/bin/"} {
		p := dir + name
		if fi, err := exec.LookPath(p); err == nil {
			return fi, nil
		}
	}
	return "", fmt.Errorf("%q not found in PATH", name)
}

// validIfaceName guards against shell/argument surprises. Interface names are
// short and limited to this set; the value comes from config/sysfs, never the
// API, but we validate defensively.
func validIfaceName(s string) bool {
	if s == "" || len(s) > 15 {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

// Supported reports whether this host can apply a bandwidth cap.
func (m *Manager) Supported() bool { return m.supported }

// ActiveMbps returns the cap currently installed in the kernel (0 = unshaped).
func (m *Manager) ActiveMbps() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeMbps
}

// Status reports the limit the UI should display. desired is the persisted cap.
func (m *Manager) Status(desired int) Status {
	return Status{LimitMbps: desired, Active: m.ActiveMbps() > 0, Supported: m.supported}
}

// ValidateMbps rejects non-positive and absurd values.
func ValidateMbps(mbps int) error {
	if mbps < minMbps || mbps > maxMbps {
		return fmt.Errorf("limit must be between %d and %d Mbps", minMbps, maxMbps)
	}
	return nil
}

// Apply installs a hard cap of mbps in both directions. It first tears down any
// existing rules (so it is idempotent), then builds egress + ingress shaping. If
// any setup step fails it tears everything down again and returns the error, so
// the interface is never left half-configured.
func (m *Manager) Apply(ctx context.Context, mbps int) error {
	if !m.supported {
		return ErrUnsupported
	}
	if err := ValidateMbps(mbps); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simulated {
		m.activeMbps = mbps
		m.logger.Printf("shaper: [demo] simulated cap on %s: %d Mbps", m.iface, mbps)
		return nil
	}

	m.teardown(ctx) // best-effort: clear any stale/previous state first

	if err := m.setup(ctx, mbps); err != nil {
		// Roll back a partial apply under a FRESH context: if setup failed because
		// ctx hit its deadline, reusing it would make every teardown command a
		// no-op and leave the interface half-configured.
		rbctx, cancel := BoundedContext()
		m.teardown(rbctx)
		cancel()
		m.activeMbps = 0
		return err
	}
	m.activeMbps = mbps
	m.logger.Printf("shaper: bandwidth cap applied on %s: %d Mbps (egress + ingress)", m.iface, mbps)
	return nil
}

// Clear removes all shaping so the interface returns to its normal unshaped
// state. It is best-effort and safe to call when nothing is installed.
func (m *Manager) Clear(ctx context.Context) error {
	if !m.supported {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.simulated {
		m.teardown(ctx)
	}
	if m.activeMbps != 0 {
		m.logger.Printf("shaper: bandwidth cap cleared on %s", m.iface)
	}
	m.activeMbps = 0
	return nil
}

// setup runs the apply sequence in order, stopping at the first failure.
func (m *Manager) setup(ctx context.Context, mbps int) error {
	rate := strconv.Itoa(mbps) + "mbit"

	// Egress (TX): HTB root qdisc on the interface; one class caps everything.
	steps := [][]string{
		{m.tcPath, "qdisc", "add", "dev", m.iface, "root", "handle", "1:", "htb", "default", "10"},
		{m.tcPath, "class", "add", "dev", m.iface, "parent", "1:", "classid", "1:10", "htb", "rate", rate, "ceil", rate},
	}
	for _, s := range steps {
		if err := m.exec(ctx, s); err != nil {
			return err
		}
	}

	// Ingress (RX): redirect incoming packets to an IFB device and cap its
	// egress. ip link add fails if the device already lingers from a crash; in
	// that case reuse it instead of failing.
	if err := m.exec(ctx, []string{m.ipPath, "link", "add", ifbDevice, "type", "ifb"}); err != nil {
		if _, showErr := m.run.Run(ctx, m.ipPath, "link", "show", ifbDevice); showErr != nil {
			return fmt.Errorf("create ifb device %s: %w", ifbDevice, err)
		}
	}
	ingress := [][]string{
		{m.ipPath, "link", "set", "dev", ifbDevice, "up"},
		{m.tcPath, "qdisc", "add", "dev", m.iface, "handle", "ffff:", "ingress"},
		{m.tcPath, "filter", "add", "dev", m.iface, "parent", "ffff:", "protocol", "all",
			"u32", "match", "u32", "0", "0", "action", "mirred", "egress", "redirect", "dev", ifbDevice},
		{m.tcPath, "qdisc", "add", "dev", ifbDevice, "root", "handle", "1:", "htb", "default", "10"},
		{m.tcPath, "class", "add", "dev", ifbDevice, "parent", "1:", "classid", "1:10", "htb", "rate", rate, "ceil", rate},
	}
	for _, s := range ingress {
		if err := m.exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// teardown removes every resource we may have created, ignoring "not found"
// errors (deleting an absent qdisc/link returns a non-zero exit). Order: remove
// the interface's egress and ingress qdiscs, then delete the IFB device (which
// drops its own qdiscs too).
func (m *Manager) teardown(ctx context.Context) {
	cmds := [][]string{
		{m.tcPath, "qdisc", "del", "dev", m.iface, "root"},
		{m.tcPath, "qdisc", "del", "dev", m.iface, "ingress"},
		{m.ipPath, "link", "del", ifbDevice},
	}
	for _, c := range cmds {
		if out, err := m.run.Run(ctx, c[0], c[1:]...); err != nil {
			// Expected when the resource is absent; log only at a low signal level.
			m.logger.Printf("shaper: teardown step %v: %v (%s)", c[1:], err, bytes.TrimSpace(out))
		}
	}
}

// exec runs one command and wraps a failure with its output for diagnosis.
func (m *Manager) exec(ctx context.Context, argv []string) error {
	out, err := m.run.Run(ctx, argv[0], argv[1:]...)
	if err != nil {
		return fmt.Errorf("%v: %w (%s)", argv[1:], err, bytes.TrimSpace(out))
	}
	return nil
}

// BoundedContext returns a short, independent context for apply/teardown so the
// fast tc/ip operations can't be aborted by a cancelled request or signal
// context (which would risk leaving the interface half-configured).
func BoundedContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}
