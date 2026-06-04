<template>
  <div class="clay p-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-sm font-semibold text-ink">History</h2>
        <p class="mt-0.5 text-xs text-muted">{{ rangeHint }}</p>
      </div>
      <SelectButton
        v-model="range"
        :options="ranges"
        option-label="label"
        option-value="value"
        :allow-empty="false"
        aria-label="History range"
        size="small"
      />
    </div>

    <div class="relative mt-4 h-72">
      <div
        v-if="loading"
        class="absolute inset-0 z-10 grid place-items-center rounded-2xl"
        style="background: color-mix(in srgb, var(--surface) 70%, transparent)"
      >
        <i class="pi pi-spin pi-spinner text-brand" />
      </div>
      <div
        v-else-if="!buckets.length"
        class="absolute inset-0 grid place-items-center text-sm text-muted"
      >
        <div class="text-center">
          <i class="pi pi-chart-bar mb-2 block text-2xl opacity-60" />
          No data for this range yet.
        </div>
      </div>
      <Chart
        v-show="buckets.length"
        type="bar"
        :data="chartData"
        :options="chartOptions"
        class="h-full w-full"
        aria-label="Traffic history chart"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Bucket } from '~/composables/useApi'

const { getHistory } = useApi()
const { colors } = useChart()
const { formatBytes } = useFormat()
const { tz } = useDisplayTz()

const ranges = [
  { label: '5 min', value: '5min', count: 48 },
  { label: 'Hourly', value: 'hour', count: 24 },
  { label: 'Daily', value: 'day', count: 30 },
  { label: 'Monthly', value: 'month', count: 12 },
]
const range = ref('hour')
const buckets = ref<Bucket[]>([])
const loading = ref(true)

const rangeHint = computed(
  () =>
    ({
      '5min': 'Last 4 hours, 5-minute resolution',
      hour: 'Last 24 hours, hourly',
      day: 'Last 30 days',
      month: 'Last 12 months',
    })[range.value],
)

function countFor(r: string): number {
  return ranges.find((x) => x.value === r)?.count ?? 24
}

function fmtLabel(ts: number, r: string): string {
  const d = new Date(ts * 1000)
  // Format in the user-selected display timezone so every chart and the footer
  // share one clock. (Day/month buckets are grouped server-side by TM_TZ; for an
  // exact match set TM_TZ to the same zone — see the README's known limitation.)
  const zone = tz.value
  switch (r) {
    case '5min':
    case 'hour':
      return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', timeZone: zone })
    case 'day':
      return d.toLocaleDateString([], { month: 'short', day: 'numeric', timeZone: zone })
    case 'month':
      return d.toLocaleDateString([], { month: 'short', year: '2-digit', timeZone: zone })
  }
  return ''
}

const chartData = computed(() => ({
  labels: buckets.value.map((b) => fmtLabel(b.ts, range.value)),
  datasets: [
    {
      label: 'Download',
      data: buckets.value.map((b) => Number(b.rx)),
      backgroundColor: colors.rx,
      borderRadius: 4,
      maxBarThickness: 28,
    },
    {
      label: 'Upload',
      data: buckets.value.map((b) => Number(b.tx)),
      backgroundColor: colors.tx,
      borderRadius: 4,
      maxBarThickness: 28,
    },
  ],
}))

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: { duration: 250 },
  interaction: { intersect: false, mode: 'index' as const },
  scales: {
    x: { grid: { display: false }, ticks: { color: colors.ticks, maxRotation: 0, autoSkip: true, maxTicksLimit: 12, font: { family: colors.font } } },
    y: {
      beginAtZero: true,
      grid: { color: colors.grid },
      ticks: { color: colors.ticks, font: { family: colors.font }, callback: (v: number) => formatBytes(Number(v)) },
    },
  },
  plugins: {
    legend: { position: 'top' as const, align: 'end' as const, labels: { color: colors.ticks, boxWidth: 10, boxHeight: 10, usePointStyle: true, font: { family: colors.font } } },
    tooltip: {
      backgroundColor: colors.tooltipBg,
      borderColor: colors.tooltipBorder,
      borderWidth: 1,
      titleColor: colors.tooltipText,
      bodyColor: colors.tooltipText,
      padding: 10,
      callbacks: { label: (ctx: any) => ` ${ctx.dataset.label}: ${formatBytes(Number(ctx.parsed.y))}` },
    },
  },
}))

let timer: ReturnType<typeof setInterval> | null = null

async function load() {
  try {
    const res = await getHistory(range.value, countFor(range.value))
    buckets.value = res.buckets || []
  } catch {
    buckets.value = []
  } finally {
    loading.value = false
  }
}

watch(range, () => {
  loading.value = true
  load()
})

onMounted(() => {
  load()
  timer = setInterval(load, 15000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>
