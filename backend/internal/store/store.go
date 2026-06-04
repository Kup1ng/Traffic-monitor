// Package store is the SQLite persistence layer. It keeps the cumulative totals
// and the durable recovery anchor in a single state row, plus time-bucketed
// rollups (hourly forever, 5-minute rolling 24h). All writes go through a single
// connection so access is serialized; with WAL and our very low write rate this
// is simple and lock-free in practice.
package store

import (
	_ "embed"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no cgo) — enables a fully static binary
)

//go:embed schema.sql
var schemaSQL string

// Store wraps the database handle.
type Store struct {
	db   *sql.DB
	path string
}

// State is the cumulative state plus the recovery anchor.
type State struct {
	RXTotal        uint64
	TXTotal        uint64
	LastRawRX      uint64
	LastRawTX      uint64
	BootID         string
	Iface          string
	InstallUnix    int64
	LastUpdateUnix int64
}

// Bucket is one time-bucketed data point.
type Bucket struct {
	TS int64  `json:"ts"`
	RX uint64 `json:"rx"`
	TX uint64 `json:"tx"`
}

// FlushArgs describes one atomic durability step: add a delta to the totals and
// the current buckets, advance the anchor, and prune old 5-minute rows.
type FlushArgs struct {
	DeltaRX, DeltaTX uint64 // bytes to add to totals and to the current buckets
	RawRX, RawTX     uint64 // current raw counter -> new durable anchor
	BootID           string // current boot_id
	NowUnix          int64  // wall clock for last_update_unix
	HourKey          int64  // UTC start-of-hour the delta belongs to
	FiveMinKey       int64  // UTC start-of-5-minute the delta belongs to
	PruneBefore      int64  // delete fivemin rows with ts_5min < this (0 = skip)
}

// Open opens (creating if needed) the database at path, sets pragmas, and
// applies the schema.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	dsn := "file:" + path + "?_pragma=journal_mode(wal)&_pragma=synchronous(normal)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Serialize all access through one connection: trivial for our write rate and
	// eliminates lock contention entirely.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

// Close flushes the WAL and closes the database.
func (s *Store) Close() error {
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return s.db.Close()
}

// Checkpoint truncates the WAL (useful on graceful shutdown).
func (s *Store) Checkpoint() error {
	_, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

// LoadState returns the state row. The bool is false when no row exists yet
// (first run).
func (s *Store) LoadState() (*State, bool, error) {
	var st State
	var rxT, txT, lrx, ltx int64
	err := s.db.QueryRow(`SELECT rx_total, tx_total, last_raw_rx, last_raw_tx, boot_id, iface, install_unix, last_update_unix FROM state WHERE id = 1`).
		Scan(&rxT, &txT, &lrx, &ltx, &st.BootID, &st.Iface, &st.InstallUnix, &st.LastUpdateUnix)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	st.RXTotal, st.TXTotal = uint64(rxT), uint64(txT)
	st.LastRawRX, st.LastRawTX = uint64(lrx), uint64(ltx)
	return &st, true, nil
}

// InitState seeds the state row on first run (totals = 0, anchor = current raw
// counter, so pre-install traffic is not counted).
func (s *Store) InitState(installUnix int64, iface, bootID string, rawRX, rawTX uint64) error {
	_, err := s.db.Exec(
		`INSERT INTO state(id, rx_total, tx_total, last_raw_rx, last_raw_tx, boot_id, iface, install_unix, last_update_unix, schema_version)
		 VALUES(1, 0, 0, ?, ?, ?, ?, ?, ?, 1)`,
		int64(rawRX), int64(rawTX), bootID, iface, installUnix, installUnix)
	return err
}

// Flush applies one atomic durability step in a single transaction: totals,
// buckets, anchor, and prune all move together (or not at all).
func (s *Store) Flush(a FlushArgs) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit

	if _, err := tx.Exec(
		`UPDATE state SET rx_total = rx_total + ?, tx_total = tx_total + ?,
		        last_raw_rx = ?, last_raw_tx = ?, boot_id = ?, last_update_unix = ?
		 WHERE id = 1`,
		int64(a.DeltaRX), int64(a.DeltaTX), int64(a.RawRX), int64(a.RawTX), a.BootID, a.NowUnix); err != nil {
		return err
	}

	if a.DeltaRX != 0 || a.DeltaTX != 0 {
		if _, err := tx.Exec(
			`INSERT INTO hourly(ts_hour, rx, tx) VALUES(?, ?, ?)
			 ON CONFLICT(ts_hour) DO UPDATE SET rx = rx + excluded.rx, tx = tx + excluded.tx`,
			a.HourKey, int64(a.DeltaRX), int64(a.DeltaTX)); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO fivemin(ts_5min, rx, tx) VALUES(?, ?, ?)
			 ON CONFLICT(ts_5min) DO UPDATE SET rx = rx + excluded.rx, tx = tx + excluded.tx`,
			a.FiveMinKey, int64(a.DeltaRX), int64(a.DeltaTX)); err != nil {
			return err
		}
	}

	if a.PruneBefore > 0 {
		if _, err := tx.Exec(`DELETE FROM fivemin WHERE ts_5min < ?`, a.PruneBefore); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// HourlyRange returns hourly buckets with from <= ts_hour < to (ascending).
func (s *Store) HourlyRange(from, to int64) ([]Bucket, error) {
	return s.rangeQuery("hourly", "ts_hour", from, to)
}

// FiveMinRange returns 5-minute buckets with from <= ts_5min < to (ascending).
func (s *Store) FiveMinRange(from, to int64) ([]Bucket, error) {
	return s.rangeQuery("fivemin", "ts_5min", from, to)
}

// rangeQuery is shared by the bucket range readers. table/col are internal
// constants (never user input).
func (s *Store) rangeQuery(table, col string, from, to int64) ([]Bucket, error) {
	q := fmt.Sprintf(`SELECT %s, rx, tx FROM %s WHERE %s >= ? AND %s < ? ORDER BY %s ASC`, col, table, col, col, col)
	rows, err := s.db.Query(q, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Bucket, 0, 64)
	for rows.Next() {
		var ts, rx, tx int64
		if err := rows.Scan(&ts, &rx, &tx); err != nil {
			return nil, err
		}
		out = append(out, Bucket{TS: ts, RX: uint64(rx), TX: uint64(tx)})
	}
	return out, rows.Err()
}
