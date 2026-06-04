// Decimal (SI, powers of 1000) byte formatting and bits/s speed formatting.
// Numbers are rendered with the .tnum class elsewhere for tabular figures.
export function useFormat() {
  const { tz } = useDisplayTz()
  const toNum = (v: number | string): number => (typeof v === 'string' ? Number(v) : v)

  function scale(n: number, units: string[]): { value: string; unit: string } {
    if (!isFinite(n) || n <= 0) return { value: '0', unit: units[0] }
    let i = 0
    let v = n
    while (v >= 1000 && i < units.length - 1) {
      v /= 1000
      i++
    }
    let digits = i === 0 ? 0 : v >= 100 ? 1 : 2
    // Rounding can push e.g. 999.99 up to 1000; carry to the next unit so we
    // show "1.00 MB" instead of "1000.0 KB".
    if (Number(v.toFixed(digits)) >= 1000 && i < units.length - 1) {
      v /= 1000
      i++
      digits = v >= 100 ? 1 : 2
    }
    return { value: v.toFixed(digits), unit: units[i] }
  }

  const byteUnits = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB']

  // bytesParts returns the value and unit separately so the UI can style them.
  function bytesParts(value: number | string): { value: string; unit: string } {
    return scale(toNum(value), byteUnits)
  }

  function formatBytes(value: number | string): string {
    const p = bytesParts(value)
    return `${p.value} ${p.unit}`
  }

  function speedParts(bps: number): { value: string; unit: string } {
    return scale(bps, ['bps', 'Kbps', 'Mbps', 'Gbps', 'Tbps'])
  }

  function formatSpeed(bps: number): string {
    const p = speedParts(bps)
    return `${p.value} ${p.unit}`
  }

  // Relative "time since" for the install date.
  function formatSince(unix: number): string {
    if (!unix) return '—'
    const secs = Math.max(0, Math.floor(Date.now() / 1000) - unix)
    const d = Math.floor(secs / 86400)
    const h = Math.floor((secs % 86400) / 3600)
    const m = Math.floor((secs % 3600) / 60)
    if (d > 0) return `${d}d ${h}h`
    if (h > 0) return `${h}h ${m}m`
    if (m > 0) return `${m}m`
    return `${secs}s`
  }

  function formatDate(unix: number): string {
    if (!unix) return '—'
    return new Date(unix * 1000).toLocaleString([], { timeZone: tz.value })
  }

  return { bytesParts, formatBytes, speedParts, formatSpeed, formatSince, formatDate }
}
