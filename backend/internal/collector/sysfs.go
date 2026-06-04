// Package collector reads network interface byte counters and related system
// state. On Linux the real source is sysfs (/sys/class/net/<iface>/statistics);
// a synthetic source (DemoReader) and a deterministic test source (FakeReader)
// implement the same CounterReader interface so the engine and UI can run on any
// OS.
package collector

import (
	"errors"
	"math"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
)

// ReadCounters returns the cumulative rx/tx byte counters for iface. These are
// monotonic since boot and reset to zero on reboot.
func ReadCounters(iface string) (rx, tx uint64, err error) {
	base := "/sys/class/net/" + iface + "/statistics/"
	rx, err = readUint64File(base + "rx_bytes")
	if err != nil {
		return 0, 0, err
	}
	tx, err = readUint64File(base + "tx_bytes")
	if err != nil {
		return 0, 0, err
	}
	return rx, tx, nil
}

// ReadBootID returns a token that changes whenever the machine reboots (and the
// kernel counters reset to zero). It prefers the kernel boot_id and falls back
// to the boot time from /proc/stat. Returns "" when neither is available (e.g.
// on non-Linux hosts), which the engine treats as "unknown" (no false reboots).
func ReadBootID() string {
	if b, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	if b, err := os.ReadFile("/proc/stat"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "btime ") {
				return "btime:" + strings.TrimSpace(strings.TrimPrefix(line, "btime "))
			}
		}
	}
	return ""
}

// ListInterfaces returns the names of all network interfaces known to the
// kernel (from sysfs), sorted alphabetically.
func ListInterfaces() ([]string, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

// DefaultRouteInterface returns the interface carrying the IPv4 default route
// (lowest metric) by parsing /proc/net/route.
func DefaultRouteInterface() (string, error) {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return "", err
	}
	best := ""
	bestMetric := int64(math.MaxInt64)
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // header / blank
		}
		f := strings.Fields(line)
		if len(f) < 11 {
			continue
		}
		if f[1] != "00000000" { // Destination != 0.0.0.0
			continue
		}
		flags, _ := strconv.ParseInt(f[3], 16, 64)
		if flags&0x1 == 0 { // RTF_UP not set
			continue
		}
		metric, _ := strconv.ParseInt(f[6], 10, 64)
		if metric < bestMetric {
			bestMetric = metric
			best = f[0]
		}
	}
	if best == "" {
		return "", errors.New("no default route found")
	}
	return best, nil
}

// ResolveInterface determines which interface to monitor: the configured name
// if given, otherwise the default-route interface, otherwise the first
// non-loopback interface.
func ResolveInterface(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	if iface, err := DefaultRouteInterface(); err == nil {
		return iface, nil
	}
	ifaces, err := ListInterfaces()
	if err != nil {
		return "", err
	}
	for _, name := range ifaces {
		if name != "lo" {
			return name, nil
		}
	}
	return "", errors.New("could not resolve an interface to monitor; set TM_INTERFACE or -iface")
}

// InterfaceInfo is static, human-facing metadata about an interface.
type InterfaceInfo struct {
	Name      string   `json:"name"`
	MAC       string   `json:"mac"`
	MTU       int      `json:"mtu"`
	SpeedMbps int      `json:"speed_mbps"` // -1 when unknown
	OperState string   `json:"operstate"`  // up | down | unknown
	Addrs     []string `json:"addrs"`
}

// GetInterfaceInfo gathers interface metadata on a best-effort basis. It uses
// the cross-platform net package and supplements with sysfs fields on Linux, so
// it degrades gracefully on other OSes / in demo mode.
func GetInterfaceInfo(iface string) InterfaceInfo {
	info := InterfaceInfo{Name: iface, SpeedMbps: -1, OperState: "unknown", Addrs: []string{}}

	if ni, err := net.InterfaceByName(iface); err == nil {
		info.MTU = ni.MTU
		info.MAC = ni.HardwareAddr.String()
		if ni.Flags&net.FlagUp != 0 {
			info.OperState = "up"
		} else {
			info.OperState = "down"
		}
		if addrs, err := ni.Addrs(); err == nil {
			for _, a := range addrs {
				info.Addrs = append(info.Addrs, a.String())
			}
		}
	}

	base := "/sys/class/net/" + iface + "/"
	if s, err := readStringFile(base + "operstate"); err == nil && s != "" {
		info.OperState = s
	}
	if s, err := readStringFile(base + "speed"); err == nil {
		if v, err := strconv.Atoi(s); err == nil {
			info.SpeedMbps = v
		}
	}
	if info.MAC == "" {
		if s, err := readStringFile(base + "address"); err == nil {
			info.MAC = s
		}
	}
	return info
}

func readUint64File(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
}

func readStringFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
