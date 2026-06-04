import type { Sample } from './useApi'

const MAX_SAMPLES = 180

// Live throughput via Server-Sent Events. The ring is seeded from
// /api/live/recent so the chart is populated immediately, then updated on each
// pushed sample. EventSource reconnects automatically on transient errors.
export function useLive() {
  const current = useState<Sample>('liveCurrent', () => ({ ts: 0, rx_bps: 0, tx_bps: 0 }))
  const samples = useState<Sample[]>('liveSamples', () => [])
  const connected = useState<boolean>('liveConnected', () => false)

  let es: EventSource | null = null

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
    if (es || typeof EventSource === 'undefined') return
    es = new EventSource('/api/live/stream')
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
      connected.value = false // EventSource will retry on its own
    }
  }

  async function start() {
    await seed()
    connect()
  }

  function stop() {
    if (es) {
      es.close()
      es = null
    }
    connected.value = false
  }

  return { current, samples, connected, start, stop }
}
