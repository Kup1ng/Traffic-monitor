export interface TotalsResp {
  rx_total: string
  tx_total: string
  total: string
  install_unix: number
  last_update_unix: number
  interface: string
}

export interface Period {
  rx: string
  tx: string
}

export interface SummaryResp {
  all_time: Period
  last_24h: Period
  today: Period
  this_month: Period
  install_unix: number
  last_update_unix: number
}

export interface InterfaceResp {
  name: string
  mac: string
  mtu: number
  speed_mbps: number
  operstate: string
  addrs: string[]
  current_rx: number
  current_tx: number
  demo: boolean
  poll_ms: number
}

export interface Bucket {
  ts: number
  rx: string
  tx: string
}

export interface HistoryResp {
  range: string
  buckets: Bucket[]
  tz?: string // server IANA zone (empty = system local) for day/month labels
}

export interface Sample {
  ts: number
  rx_bps: number
  tx_bps: number
}

// Base path the app is served under. Vite bakes import.meta.env.BASE_URL at
// build time (a placeholder in production); the Go server rewrites it to the
// secret base path at startup. Every API/SSE URL is built relative to it.
const APP_BASE = (import.meta.env.BASE_URL || '/')

export function apiUrl(p: string): string {
  const base = APP_BASE.endsWith('/') ? APP_BASE : APP_BASE + '/'
  return base + p.replace(/^\/+/, '')
}

// Thin wrapper over $fetch. All requests are same-origin (under the base path)
// and send the session cookie. In dev, Nuxt's devProxy forwards /api to the Go
// backend.
export function useApi() {
  function request<T>(path: string, opts: Record<string, unknown> = {}): Promise<T> {
    return $fetch<T>(apiUrl(path), { credentials: 'include', ...opts })
  }

  return {
    request,
    getSession: () =>
      request<{ authenticated: boolean; password_configured: boolean }>('/api/session'),
    login: (password: string) => request('/api/login', { method: 'POST', body: { password } }),
    logout: () => request('/api/logout', { method: 'POST' }),
    getTotals: () => request<TotalsResp>('/api/totals'),
    getSummary: () => request<SummaryResp>('/api/summary'),
    getInterface: () => request<InterfaceResp>('/api/interface'),
    getRecent: () => request<Sample[]>('/api/live/recent'),
    getHistory: (range: string, count?: number) =>
      request<HistoryResp>(`/api/history?range=${range}${count ? `&count=${count}` : ''}`),
  }
}
