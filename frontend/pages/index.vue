<template>
  <div class="min-h-dvh">
    <TopBar :iface="iface" :connected="connected" />

    <main class="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6">
      <!-- Cumulative totals -->
      <section class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <KpiCard label="Total download" icon="pi pi-arrow-down" accent="rx" :parts="dl" sub="Received since install" />
        <KpiCard label="Total upload" icon="pi pi-arrow-up" accent="tx" :parts="ul" sub="Transmitted since install" />
        <KpiCard label="Grand total" icon="pi pi-database" accent="brand" :parts="total" sub="Combined RX + TX" />
      </section>

      <!-- Live + summary -->
      <section class="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <LiveThroughput class="lg:col-span-2" />
        <SummaryPanel :summary="summary" />
      </section>

      <!-- History -->
      <PeriodChart />

      <!-- Footer / interface details -->
      <footer class="flex flex-wrap items-center justify-between gap-3 border-t border-edge pt-5 text-xs text-muted">
        <div class="flex flex-wrap items-center gap-x-4 gap-y-1">
          <span v-if="iface">
            <span class="text-ink tnum">{{ iface.name }}</span>
            <template v-if="iface.mac"> · MAC {{ iface.mac }}</template>
            <template v-if="iface.mtu"> · MTU {{ iface.mtu }}</template>
            <template v-if="iface.speed_mbps > 0"> · {{ iface.speed_mbps }} Mbps link</template>
          </span>
          <span v-if="totals">Monitoring since <span class="text-ink">{{ formatDate(totals.install_unix) }}</span></span>
        </div>
        <div class="flex items-center gap-3">
          <span>v{{ version }}</span>
          <a
            href="https://github.com/Kup1ng/Traffic-monitor"
            target="_blank"
            rel="noopener"
            class="inline-flex items-center gap-1 hover:text-ink"
          >
            <i class="pi pi-github" /> GitHub
          </a>
        </div>
      </footer>
    </main>
  </div>
</template>

<script setup lang="ts">
import type { TotalsResp, SummaryResp, InterfaceResp } from '~/composables/useApi'

const { getTotals, getSummary, getInterface, request } = useApi()
const { bytesParts, formatDate } = useFormat()
const live = useLive()
const { connected } = live

const totals = ref<TotalsResp | null>(null)
const summary = ref<SummaryResp | null>(null)
const iface = ref<InterfaceResp | null>(null)
const version = ref('—')

const dl = computed(() => bytesParts(totals.value?.rx_total ?? 0))
const ul = computed(() => bytesParts(totals.value?.tx_total ?? 0))
const total = computed(() => bytesParts(totals.value?.total ?? 0))

function isUnauthorized(e: any): boolean {
  return e?.statusCode === 401 || e?.status === 401 || e?.response?.status === 401
}

async function loadAll() {
  const [t, s, i] = await Promise.allSettled([getTotals(), getSummary(), getInterface()])
  if (t.status === 'fulfilled') totals.value = t.value
  if (s.status === 'fulfilled') summary.value = s.value
  if (i.status === 'fulfilled') iface.value = i.value

  for (const r of [t, s, i]) {
    if (r.status === 'rejected' && isUnauthorized(r.reason)) {
      await navigateTo('/login')
      return
    }
  }
}

let timer: ReturnType<typeof setInterval> | null = null

onMounted(async () => {
  await loadAll()
  try {
    version.value = (await request<{ version: string }>('/api/version')).version
  } catch {
    /* non-critical */
  }
  await live.start()
  timer = setInterval(loadAll, 5000)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
  live.stop()
})
</script>
