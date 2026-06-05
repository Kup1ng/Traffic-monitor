package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strconv"
)

const (
	keyPasswordHash  = "password_hash"
	keySessionSecret = "session_secret"
	keyBWLimitMbps   = "bw_limit_mbps"
)

// GetSetting returns a setting value; the bool is false when the key is absent.
func (s *Store) GetSetting(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// SetSetting inserts or updates a setting.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// GetBandwidthLimit returns the persisted bandwidth cap in Mbps (0 = disabled,
// also returned when the value is unset or unparseable).
func (s *Store) GetBandwidthLimit() (int, error) {
	v, ok, err := s.GetSetting(keyBWLimitMbps)
	if err != nil || !ok {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, nil
	}
	return n, nil
}

// SetBandwidthLimit persists the desired bandwidth cap in Mbps (0 = disabled).
func (s *Store) SetBandwidthLimit(mbps int) error {
	if mbps < 0 {
		mbps = 0
	}
	return s.SetSetting(keyBWLimitMbps, strconv.Itoa(mbps))
}

// GetPasswordHash returns the stored admin bcrypt hash (bool false if unset).
func (s *Store) GetPasswordHash() (string, bool, error) { return s.GetSetting(keyPasswordHash) }

// SetPasswordHash stores the admin bcrypt hash.
func (s *Store) SetPasswordHash(hash string) error { return s.SetSetting(keyPasswordHash, hash) }

// GetOrCreateSessionSecret returns the persisted 32-byte session secret,
// generating and storing one on first use so sessions survive restarts.
func (s *Store) GetOrCreateSessionSecret() ([]byte, error) {
	if v, ok, err := s.GetSetting(keySessionSecret); err != nil {
		return nil, err
	} else if ok {
		return hex.DecodeString(v)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	if err := s.SetSetting(keySessionSecret, hex.EncodeToString(secret)); err != nil {
		return nil, err
	}
	return secret, nil
}

// RotateSessionSecret replaces the persisted session secret with a fresh one,
// immediately invalidating all existing session cookies.
func (s *Store) RotateSessionSecret() error {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	return s.SetSetting(keySessionSecret, hex.EncodeToString(secret))
}
