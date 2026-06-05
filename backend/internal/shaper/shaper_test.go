package shaper

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"testing"
)

// fakeRunner records every command and can be told to fail on matching ones.
type fakeRunner struct {
	calls  [][]string
	failOn func(argv []string) error
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	// Mimic exec.CommandContext: a cancelled/expired context means the command is
	// not executed at all.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rec := append([]string{name}, args...)
	f.calls = append(f.calls, rec)
	if f.failOn != nil {
		if err := f.failOn(rec); err != nil {
			return []byte("simulated failure"), err
		}
	}
	return nil, nil
}

func (f *fakeRunner) joined() []string {
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = strings.Join(c, " ")
	}
	return out
}

func testManager(fr Runner) *Manager {
	return &Manager{
		iface:     "eth0",
		run:       fr,
		tcPath:    "tc",
		ipPath:    "ip",
		logger:    log.New(io.Discard, "", 0),
		supported: true,
	}
}

func TestApplySequence(t *testing.T) {
	fr := &fakeRunner{}
	m := testManager(fr)
	if err := m.Apply(context.Background(), 50); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if m.ActiveMbps() != 50 {
		t.Fatalf("activeMbps = %d, want 50", m.ActiveMbps())
	}

	got := fr.joined()
	want := []string{
		// teardown-first (idempotent)
		"tc qdisc del dev eth0 root",
		"tc qdisc del dev eth0 ingress",
		"ip link del tm-ifb0",
		// egress
		"tc qdisc add dev eth0 root handle 1: htb default 10",
		"tc class add dev eth0 parent 1: classid 1:10 htb rate 50mbit ceil 50mbit",
		// ingress via ifb
		"ip link add tm-ifb0 type ifb",
		"ip link set dev tm-ifb0 up",
		"tc qdisc add dev eth0 handle ffff: ingress",
		"tc filter add dev eth0 parent ffff: protocol all u32 match u32 0 0 action mirred egress redirect dev tm-ifb0",
		"tc qdisc add dev tm-ifb0 root handle 1: htb default 10",
		"tc class add dev tm-ifb0 parent 1: classid 1:10 htb rate 50mbit ceil 50mbit",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d commands, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("command %d:\n got: %s\nwant: %s", i, got[i], want[i])
		}
	}
}

func TestApplyRollsBackOnFailure(t *testing.T) {
	fr := &fakeRunner{failOn: func(argv []string) error {
		joined := strings.Join(argv, " ")
		if strings.Contains(joined, "filter add") {
			return errors.New("filter rejected")
		}
		return nil
	}}
	m := testManager(fr)
	err := m.Apply(context.Background(), 100)
	if err == nil {
		t.Fatal("expected Apply to fail")
	}
	if m.ActiveMbps() != 0 {
		t.Fatalf("activeMbps = %d after failed apply, want 0", m.ActiveMbps())
	}
	// The last three commands must be the rollback teardown so the interface is
	// never left half-configured.
	got := fr.joined()
	tail := got[len(got)-3:]
	wantTail := []string{
		"tc qdisc del dev eth0 root",
		"tc qdisc del dev eth0 ingress",
		"ip link del tm-ifb0",
	}
	for i := range wantTail {
		if tail[i] != wantTail[i] {
			t.Errorf("rollback command %d: got %q want %q", i, tail[i], wantTail[i])
		}
	}
}

func TestRollbackUsesFreshContext(t *testing.T) {
	fr := &fakeRunner{}
	m := testManager(fr)
	// An already-expired context: setup can't run, and a naive rollback that
	// reused this context couldn't run either. The rollback must use a fresh one
	// so the interface is never left half-configured.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := m.Apply(ctx, 50); err == nil {
		t.Fatal("expected Apply to fail with an expired context")
	}
	if m.ActiveMbps() != 0 {
		t.Fatalf("activeMbps = %d, want 0", m.ActiveMbps())
	}
	// Only the rollback teardown should have actually executed (the pre-clear and
	// setup commands no-op'd under the dead context); its three dels prove the
	// rollback ran under a fresh context.
	got := fr.joined()
	want := []string{
		"tc qdisc del dev eth0 root",
		"tc qdisc del dev eth0 ingress",
		"ip link del tm-ifb0",
	}
	if len(got) != len(want) {
		t.Fatalf("rollback ran %d commands, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rollback command %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestClearSequence(t *testing.T) {
	fr := &fakeRunner{}
	m := testManager(fr)
	m.activeMbps = 50
	if err := m.Clear(context.Background()); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if m.ActiveMbps() != 0 {
		t.Fatalf("activeMbps = %d after Clear, want 0", m.ActiveMbps())
	}
	want := []string{
		"tc qdisc del dev eth0 root",
		"tc qdisc del dev eth0 ingress",
		"ip link del tm-ifb0",
	}
	got := fr.joined()
	if len(got) != len(want) {
		t.Fatalf("Clear ran %d commands, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("clear command %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestIfbReusedWhenAddFails(t *testing.T) {
	// ip link add fails (stale device) but ip link show succeeds -> setup proceeds.
	fr := &fakeRunner{failOn: func(argv []string) error {
		j := strings.Join(argv, " ")
		if strings.Contains(j, "link add tm-ifb0") {
			return errors.New("RTNETLINK answers: File exists")
		}
		return nil
	}}
	m := testManager(fr)
	if err := m.Apply(context.Background(), 10); err != nil {
		t.Fatalf("Apply should tolerate an existing ifb device, got: %v", err)
	}
	if m.ActiveMbps() != 10 {
		t.Fatalf("activeMbps = %d, want 10", m.ActiveMbps())
	}
}

func TestUnsupportedNoOps(t *testing.T) {
	fr := &fakeRunner{}
	m := testManager(fr)
	m.supported = false

	if err := m.Apply(context.Background(), 50); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Apply on unsupported host: got %v, want ErrUnsupported", err)
	}
	if err := m.Clear(context.Background()); err != nil {
		t.Fatalf("Clear on unsupported host should be nil, got %v", err)
	}
	if len(fr.calls) != 0 {
		t.Fatalf("unsupported manager ran %d commands, want 0: %v", len(fr.calls), fr.calls)
	}
}

func TestSimulatedRunsNoCommands(t *testing.T) {
	fr := &fakeRunner{}
	m := &Manager{iface: "demo0", run: fr, logger: log.New(io.Discard, "", 0), supported: true, simulated: true}

	if err := m.Apply(context.Background(), 50); err != nil {
		t.Fatalf("simulated Apply: %v", err)
	}
	if m.ActiveMbps() != 50 {
		t.Fatalf("activeMbps = %d, want 50", m.ActiveMbps())
	}
	if err := m.Clear(context.Background()); err != nil {
		t.Fatalf("simulated Clear: %v", err)
	}
	if m.ActiveMbps() != 0 {
		t.Fatalf("activeMbps = %d after clear, want 0", m.ActiveMbps())
	}
	if len(fr.calls) != 0 {
		t.Fatalf("simulated manager ran %d commands, want 0: %v", len(fr.calls), fr.calls)
	}
	// Validation still applies in simulated mode.
	if err := m.Apply(context.Background(), 0); err == nil {
		t.Fatal("expected validation error for 0 Mbps in simulated mode")
	}
}

func TestValidateMbps(t *testing.T) {
	for _, tc := range []struct {
		mbps int
		ok   bool
	}{{0, false}, {-5, false}, {1, true}, {100, true}, {1_000_000, true}, {1_000_001, false}} {
		err := ValidateMbps(tc.mbps)
		if (err == nil) != tc.ok {
			t.Errorf("ValidateMbps(%d): err=%v, wantOK=%v", tc.mbps, err, tc.ok)
		}
	}
}
