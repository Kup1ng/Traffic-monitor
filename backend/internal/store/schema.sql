-- Single authoritative state row. Carries the cumulative totals and the durable
-- recovery anchor (last raw counter + boot_id) used to survive reboots,
-- restarts, and crashes without losing or double-counting bytes.
CREATE TABLE IF NOT EXISTS state (
  id               INTEGER PRIMARY KEY CHECK (id = 1),
  rx_total         INTEGER NOT NULL DEFAULT 0,   -- cumulative received bytes since install
  tx_total         INTEGER NOT NULL DEFAULT 0,   -- cumulative transmitted bytes since install
  last_raw_rx      INTEGER NOT NULL DEFAULT 0,   -- raw counter at last durable flush (anchor)
  last_raw_tx      INTEGER NOT NULL DEFAULT 0,
  boot_id          TEXT    NOT NULL DEFAULT '',  -- kernel boot_id at last flush (reboot detection)
  iface            TEXT    NOT NULL DEFAULT '',  -- monitored interface (guards against NIC swap)
  install_unix     INTEGER NOT NULL,             -- first-run timestamp
  last_update_unix INTEGER NOT NULL,             -- last flush timestamp
  schema_version   INTEGER NOT NULL DEFAULT 1
);

-- Finest granularity persisted forever: one row per UTC hour (~8760 rows/year).
-- Daily and monthly views are derived from this on read using the configured
-- timezone, so changing the timezone never corrupts history.
CREATE TABLE IF NOT EXISTS hourly (
  ts_hour INTEGER PRIMARY KEY,                   -- unix seconds, truncated to the UTC hour
  rx      INTEGER NOT NULL DEFAULT 0,
  tx      INTEGER NOT NULL DEFAULT 0
);

-- Rolling fine-grained recent history: one row per UTC 5-minute bucket, pruned
-- to the last 24 hours (~288 rows). Survives restarts for detailed recent zoom.
CREATE TABLE IF NOT EXISTS fivemin (
  ts_5min INTEGER PRIMARY KEY,                   -- unix seconds, truncated to a UTC 5-minute boundary
  rx      INTEGER NOT NULL DEFAULT 0,
  tx      INTEGER NOT NULL DEFAULT 0
);

-- Key/value secrets and settings (admin password bcrypt hash, session secret).
CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
