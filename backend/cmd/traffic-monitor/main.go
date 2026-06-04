// Command traffic-monitor is a lightweight, self-hosted network traffic monitor.
//
// Subcommands:
//
//	serve            run the monitor and web server (default)
//	set-password     set the admin web-panel password
//	reset-password   alias for set-password
//	version          print the build version
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "time/tzdata" // embed the IANA timezone database for TM_TZ on any host

	"github.com/Kup1ng/Traffic-monitor/internal/api"
	"github.com/Kup1ng/Traffic-monitor/internal/auth"
	"github.com/Kup1ng/Traffic-monitor/internal/collector"
	"github.com/Kup1ng/Traffic-monitor/internal/config"
	"github.com/Kup1ng/Traffic-monitor/internal/engine"
	"github.com/Kup1ng/Traffic-monitor/internal/store"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

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
	case "set-password", "reset-password":
		if err := runSetPassword(args); err != nil {
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

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	secret, err := st.GetOrCreateSessionSecret()
	if err != nil {
		return fmt.Errorf("session secret: %w", err)
	}

	eng, err := engine.New(cfg, reader, st)
	if err != nil {
		return err
	}
	authn := auth.New(st, secret, cfg.SessionTTL, cfg.CookieSecure)

	srv, err := api.NewServer(cfg, eng, authn, version)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Run the accounting engine; it performs a final flush on ctx cancellation.
	engDone := make(chan struct{})
	go func() {
		_ = eng.Run(ctx)
		close(engDone)
	}()

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	log.Printf("traffic-monitor %s listening on %s (interface %q, demo=%v)", version, cfg.Listen, eng.Iface(), cfg.Demo)
	if !authn.PasswordConfigured() {
		log.Printf("WARNING: no admin password set — run 'traffic-monitor set-password' (or reinstall) before exposing the panel")
	}

	err = httpSrv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	<-engDone // wait for the engine's final flush
	return err
}

// runSetPassword handles the set-password / reset-password subcommands. The
// password comes from --password or, by default, one line on stdin (install.sh
// pipes it via --stdin so it never appears in the process list).
func runSetPassword(args []string) error {
	fs := flag.NewFlagSet("set-password", flag.ContinueOnError)
	dbPath := fs.String("db", envOr("TM_DB", "traffic.db"), "path to the SQLite database file")
	pw := fs.String("password", "", "new password (omit to read one line from stdin)")
	fs.Bool("stdin", false, "read the password from stdin (the default when --password is absent)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	password := *pw
	if password == "" {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return fmt.Errorf("read password from stdin: %w", err)
		}
		password = strings.TrimRight(line, "\r\n")
	}
	if password == "" {
		return errors.New("password must not be empty")
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := auth.SetPassword(st, password); err != nil {
		return err
	}
	fmt.Println("admin password updated")
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func usage() {
	fmt.Printf(`traffic-monitor %s

Usage:
  traffic-monitor [serve] [flags]      run the monitor and web server
  traffic-monitor set-password [flags] set the admin web-panel password
  traffic-monitor reset-password       alias for set-password
  traffic-monitor version              print the build version
  traffic-monitor help                 show this help

Run "traffic-monitor serve -h" or "traffic-monitor set-password -h" for flags.
`, version)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
