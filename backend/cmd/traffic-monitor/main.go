// Command traffic-monitor is a lightweight, self-hosted network traffic monitor.
//
// Subcommands:
//
//	serve            run the monitor and web server (default)
//	set-password     set the admin web-panel password (added with auth)
//	reset-password   alias for set-password (added with auth)
//	version          print the build version
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "time/tzdata" // embed the IANA timezone database for TM_TZ on any host

	"github.com/Kup1ng/Traffic-monitor/internal/collector"
	"github.com/Kup1ng/Traffic-monitor/internal/config"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	sub := "serve"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = args[0]
		args = args[1:]
	}

	switch sub {
	case "serve":
		if err := runServe(args); err != nil {
			fail(err)
		}
	case "version", "--version", "-v":
		fmt.Printf("traffic-monitor %s\n", version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", sub)
		usage()
		os.Exit(2)
	}
}

// runServe currently runs an interim console meter that proves the collector
// works end to end. It is replaced by the full HTTP server + storage engine in
// a later step.
func runServe(args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}

	var reader collector.CounterReader
	if cfg.Demo {
		reader = collector.NewDemoReader(cfg.Interface)
	} else {
		iface, err := collector.ResolveInterface(cfg.Interface)
		if err != nil {
			return err
		}
		cfg.Interface = iface
		reader = collector.NewSysfsReader(iface)
	}

	info := collector.GetInterfaceInfo(reader.Iface())
	fmt.Printf("traffic-monitor %s — monitoring %q (demo=%v)\n", version, reader.Iface(), cfg.Demo)
	fmt.Printf("interface: state=%s mtu=%d speed=%dMbps mac=%s\n", info.OperState, info.MTU, info.SpeedMbps, info.MAC)
	fmt.Printf("listen=%s db=%s poll=%s flush=%s\n", cfg.Listen, cfg.DBPath, cfg.PollInterval, cfg.FlushInterval)
	fmt.Println("(interim console meter — press Ctrl+C to exit)")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	lastRX, lastTX, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read counters: %w", err)
	}
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	secs := cfg.PollInterval.Seconds()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nshutting down")
			return nil
		case <-ticker.C:
			rx, tx, err := reader.Read()
			if err != nil {
				fmt.Fprintf(os.Stderr, "read error: %v\n", err)
				continue
			}
			dRX := delta(rx, lastRX)
			dTX := delta(tx, lastTX)
			lastRX, lastTX = rx, tx
			fmt.Printf("\r↓ %-12s  ↑ %-12s", bitsPerSec(dRX, secs), bitsPerSec(dTX, secs))
		}
	}
}

// delta computes bytes transferred since the previous reading, treating a
// counter that went backwards as a reset (return the new value).
func delta(cur, prev uint64) uint64 {
	if cur >= prev {
		return cur - prev
	}
	return cur
}

func bitsPerSec(bytes uint64, secs float64) string {
	bps := float64(bytes) * 8 / secs
	switch {
	case bps >= 1e9:
		return fmt.Sprintf("%.2f Gbps", bps/1e9)
	case bps >= 1e6:
		return fmt.Sprintf("%.2f Mbps", bps/1e6)
	case bps >= 1e3:
		return fmt.Sprintf("%.2f Kbps", bps/1e3)
	default:
		return fmt.Sprintf("%.0f bps", bps)
	}
}

func usage() {
	fmt.Printf(`traffic-monitor %s

Usage:
  traffic-monitor [serve] [flags]   run the monitor and web server
  traffic-monitor version           print the build version
  traffic-monitor help              show this help

Run "traffic-monitor serve -h" for the list of flags.
`, version)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
