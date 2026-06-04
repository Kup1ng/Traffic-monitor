# Traffic-monitor

A self-hosted, **extremely lightweight** network-traffic monitor for a single network
interface on Ubuntu/Linux. It ships as **one static binary** that embeds its web UI and
serves both the JSON API and the dashboard on a single port.

> Status: work in progress. See [`docs`](#documentation) below as sections land.

## Why

- **Cumulative forever.** Tracks total RX / TX / combined bytes since the moment of
  installation — multi-terabyte correct — and **keeps accumulating across reboots**.
- **Full long-term history.** Per-hour data kept *forever*, plus 5-minute resolution for the
  last 24 hours and daily/monthly views derived on read — far more complete than `vnstat`'s
  ~30-day window.
- **Lossless.** A durable raw-counter anchor plus kernel `boot_id` detection means bytes are
  never lost or double-counted across reboots, restarts, or crashes.
- **Tiny footprint.** Pure-Go, near-zero idle CPU/RAM, a single SQLite file, no heavy
  runtime dependencies, and a fast-loading dashboard.

## Tech stack

| Layer | Choice |
|-------|--------|
| Backend | Go (single static `linux/amd64` binary, `CGO_ENABLED=0`) |
| Storage | SQLite via pure-Go `modernc.org/sqlite` (WAL) |
| Frontend | Nuxt 3 (SPA) + Tailwind CSS + PrimeVue v4 + Chart.js |
| Packaging | Frontend embedded into the binary with `//go:embed` |
| Service | systemd unit (auto-start, restart on failure) |

## Documentation

- Build & local development — _TBD_
- Release flow (version tags → GitHub Release) — _TBD_
- Server install / update / uninstall / reset-password — _TBD_
- Configuration reference — _TBD_
- API reference — _TBD_
- Security notes — _TBD_

## License

[MIT](./LICENSE)
