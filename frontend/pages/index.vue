<template>
  <div class="min-h-dvh">
    <TopBar :iface="iface" :connected="connected" />

    <main class="mx-auto max-w-7xl space-y-5 px-3 py-5 sm:space-y-6 sm:px-6 sm:py-6">
      <!-- Cumulative totals -->
      <section class="grid grid-cols-1 gap-4 sm:gap-5 md:grid-cols-3">
        <KpiCard label="Total download" icon="pi pi-arrow-down" accent="rx" :parts="dl" sub="Received since install" />
        <KpiCard label="Total upload" icon="pi pi-arrow-up" accent="tx" :parts="ul" sub="Transmitted since install" />
        <KpiCard label="Grand total" icon="pi pi-database" accent="brand" :parts="total" sub="Combined RX + TX" />
      </section>

      <!-- Live + summary -->
      <section class="grid grid-cols-1 gap-5 sm:gap-6 lg:grid-cols-3">
        <LiveThroughput class="lg:col-span-2" />
        <SummaryPanel :summary="summary" />
      </section>

      <!-- History -->
      <PeriodChart />

      <!-- Footer / interface details -->
      <footer
        class="clay-inset flex flex-col items-center gap-3 px-5 py-4 text-center text-[11px] leading-relaxed text-muted sm:flex-row sm:items-center sm:justify-between sm:text-left sm:text-xs"
      >
        <div class="flex flex-col items-center gap-1.5 sm:items-start">
          <div
            v-if="iface"
            class="flex flex-wrap items-center justify-center gap-x-2.5 gap-y-1 sm:justify-start"
          >
            <span class="rounded-md bg-[var(--surface)] px-2 py-0.5 font-semibold text-ink tnum shadow-sm">
              {{ iface.name }}
            </span>
            <span v-if="iface.mac" class="tnum">{{ iface.mac }}</span>
            <span v-if="iface.mtu">MTU {{ iface.mtu }}</span>
            <span v-if="iface.speed_mbps > 0">{{ iface.speed_mbps }} Mbps</span>
          </div>
          <div v-if="totals" class="inline-flex items-center gap-1">
            <i class="pi pi-clock text-[10px] opacity-70" />
            <span>Monitoring since <span class="text-ink">{{ formatDate(totals.install_unix) }}</span></span>
          </div>
        </div>

        <div class="flex flex-wrap items-center justify-center gap-x-3 gap-y-2 sm:justify-end">
          <div class="inline-flex items-center gap-1.5" title="Display timezone">
            <i class="pi pi-globe text-[11px] opacity-70" aria-hidden="true" />
            <Select
              :model-value="tz"
              :options="zones"
              option-label="label"
              option-value="value"
              filter
              :filter-fields="['name']"
              auto-filter-focus
              reset-filter-on-hide
              filter-placeholder="Search city or region…"
              size="small"
              class="tm-tz-select"
              aria-label="Display timezone"
              @update:model-value="setTz"
            />
          </div>
          <span class="tnum rounded-md bg-[var(--surface)] px-2 py-0.5 shadow-sm">{{ version }}</span>
          <a
            href="https://github.com/Kup1ng/Traffic-monitor"
            target="_blank"
            rel="noopener"
            class="inline-flex items-center gap-1 transition-colors hover:text-ink"
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
const { tz, zones, setTz } = useDisplayTz()
const auth = useAuth()
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
      live.stop()
      await auth.handleUnauthorized()
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
