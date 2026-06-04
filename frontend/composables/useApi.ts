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
}

export interface Sample {
  ts: number
  rx_bps: number
  tx_bps: number
}

// Thin wrapper over $fetch. All requests are same-origin and send the session
// cookie. In dev, Nuxt's devProxy forwards /api to the Go backend.
export function useApi() {
  function request<T>(path: string, opts: Record<string, unknown> = {}): Promise<T> {
    return $fetch<T>(path, { credentials: 'include', ...opts })
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
