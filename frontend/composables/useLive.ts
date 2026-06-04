import { apiUrl, type Sample } from './useApi'

const MAX_SAMPLES = 180

// Live throughput via Server-Sent Events. The ring is seeded from
// /api/live/recent so the chart is populated immediately, then updated on each
// pushed sample.
export function useLive() {
  const current = useState<Sample>('liveCurrent', () => ({ ts: 0, rx_bps: 0, tx_bps: 0 }))
  const samples = useState<Sample[]>('liveSamples', () => [])
  const connected = useState<boolean>('liveConnected', () => false)
  const auth = useAuth()

  let es: EventSource | null = null
  let stopped = false // closure flag so a navigate-away during seed() cancels connect()

  function push(s: Sample) {
    current.value = s
    const arr = samples.value.slice()
    arr.push(s)
    while (arr.length > MAX_SAMPLES) arr.shift()
    samples.value = arr
  }

  async function seed() {
    try {
      const recent = await useApi().getRecent()
      if (Array.isArray(recent) && recent.length) {
        samples.value = recent.slice(-MAX_SAMPLES)
        current.value = recent[recent.length - 1]
      }
    } catch {
      /* ignore — the stream will fill it */
    }
  }

  function connect() {
    if (es || stopped || typeof EventSource === 'undefined') return
    es = new EventSource(apiUrl('/api/live/stream'))
    es.onopen = () => {
      connected.value = true
    }
    es.onmessage = (e) => {
      try {
        push(JSON.parse(e.data) as Sample)
      } catch {
        /* ignore malformed frame */
      }
    }
    es.onerror = () => {
      connected.value = false
      // A CLOSED state is a fatal error (e.g. 401 after the session expired):
      // EventSource does NOT auto-reconnect from CLOSED. Stop and re-authenticate
      // instead of leaving the badge stuck Offline forever.
      if (es && es.readyState === EventSource.CLOSED) {
        stop()
        void auth.handleUnauthorized()
      }
      // (A CONNECTING state is a transient drop the browser retries on its own.)
    }
  }

  async function start() {
    stopped = false
    await seed()
    if (stopped) return // navigated away while seeding — don't open a leaked stream
    connect()
  }

  function stop() {
    stopped = true
    if (es) {
      es.close()
      es = null
    }
    connected.value = false
  }

  return { current, samples, connected, start, stop }
}
