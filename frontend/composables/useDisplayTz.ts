// Shared display timezone for every date/time the dashboard renders. Defaults
// to Asia/Tehran and persists the user's choice in localStorage so it survives
// reloads. The live chart, history labels, and the install date all read this
// one ref, so a single footer selector controls the whole UI's clock.
const TZ_KEY = 'tm.displayTz'
const DEFAULT_TZ = 'Asia/Tehran'

export interface TzOption {
  label: string // shown in the field/list, e.g. "(UTC+03:30)  Asia/Tehran"
  value: string // IANA id, e.g. "Asia/Tehran"
  name: string // human zone name only, e.g. "Asia/Tehran" — the field the filter searches
}

// A compact spread used only when the browser is too old to enumerate zones.
const FALLBACK_ZONES = [
  'UTC',
  'Pacific/Honolulu',
  'America/Anchorage',
  'America/Los_Angeles',
  'America/Denver',
  'America/Chicago',
  'America/New_York',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Moscow',
  'Africa/Cairo',
  'Africa/Johannesburg',
  'Asia/Tehran',
  'Asia/Dubai',
  'Asia/Karachi',
  'Asia/Kolkata',
  'Asia/Dhaka',
  'Asia/Bangkok',
  'Asia/Shanghai',
  'Asia/Singapore',
  'Asia/Hong_Kong',
  'Asia/Tokyo',
  'Asia/Seoul',
  'Australia/Sydney',
  'Pacific/Auckland',
]

let cachedZones: TzOption[] | null = null
let restored = false

function isValidZone(zone: string): boolean {
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: zone })
    return true
  } catch {
    return false
  }
}

// Current UTC offset of a zone in minutes (honours DST for "now"). Used to label
// and sort the options so the picker reads like a normal timezone list.
function offsetMinutes(zone: string, at: Date): number {
  try {
    const dtf = new Intl.DateTimeFormat('en-US', {
      timeZone: zone,
      hourCycle: 'h23',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
    const m: Record<string, number> = {}
    for (const p of dtf.formatToParts(at)) {
      if (p.type !== 'literal') m[p.type] = Number(p.value)
    }
    const asUTC = Date.UTC(m.year, (m.month || 1) - 1, m.day || 1, m.hour || 0, m.minute || 0, m.second || 0)
    return Math.round((asUTC - at.getTime()) / 60000)
  } catch {
    return 0
  }
}

function fmtOffset(min: number): string {
  const sign = min >= 0 ? '+' : '-'
  const abs = Math.abs(min)
  const h = String(Math.floor(abs / 60)).padStart(2, '0')
  const m = String(abs % 60).padStart(2, '0')
  return `UTC${sign}${h}:${m}`
}

function buildZones(): TzOption[] {
  if (cachedZones) return cachedZones
  const now = new Date()
  let names: string[] = []
  try {
    const supported = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf
    if (typeof supported === 'function') names = supported('timeZone')
  } catch {
    /* fall back below */
  }
  if (!names.length) names = FALLBACK_ZONES
  // Intl.supportedValuesOf omits a bare "UTC" — make sure it is always pickable.
  if (!names.includes('UTC')) names = ['UTC', ...names]

  const rows = names.map((zone) => {
    const off = offsetMinutes(zone, now)
    const name = zone.replace(/_/g, ' ')
    return { value: zone, off, name, label: `(${fmtOffset(off)})  ${name}` }
  })
  rows.sort((a, b) => a.off - b.off || a.value.localeCompare(b.value))
  cachedZones = rows.map(({ label, value, name }) => ({ label, value, name }))
  return cachedZones
}

export function useDisplayTz() {
  const tz = useState<string>('displayTz', () => DEFAULT_TZ)

  // Restore a saved choice once, before first paint (SPA → client only), so the
  // selected zone is in place without a flash of the default.
  if (!restored) {
    restored = true
    if (typeof localStorage !== 'undefined') {
      const saved = localStorage.getItem(TZ_KEY)
      if (saved && isValidZone(saved)) tz.value = saved
    }
  }

  const zones = computed(() => buildZones())

  function setTz(value: string) {
    if (!value || !isValidZone(value)) return
    tz.value = value
    if (typeof localStorage !== 'undefined') localStorage.setItem(TZ_KEY, value)
  }

  return { tz, zones, setTz }
}
