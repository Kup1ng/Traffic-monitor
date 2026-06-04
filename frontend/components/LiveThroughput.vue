<template>
  <div class="flex h-full flex-col clay p-6">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h2 class="text-sm font-semibold text-ink">Live throughput</h2>
        <p class="mt-0.5 text-xs text-muted">Real-time up/down speed</p>
      </div>
      <div class="flex items-center gap-6">
        <div>
          <div class="flex items-center gap-1.5 text-xs text-muted">
            <i class="pi pi-arrow-down text-[var(--rx)]" style="font-size: 0.7rem" /> Download
          </div>
          <div class="mt-0.5 flex items-baseline gap-1">
            <span class="tnum text-2xl font-semibold text-[var(--rx)]">{{ rxParts.value }}</span>
            <span class="text-xs text-muted">{{ rxParts.unit }}</span>
          </div>
        </div>
        <div>
          <div class="flex items-center gap-1.5 text-xs text-muted">
            <i class="pi pi-arrow-up text-[var(--tx)]" style="font-size: 0.7rem" /> Upload
          </div>
          <div class="mt-0.5 flex items-baseline gap-1">
            <span class="tnum text-2xl font-semibold text-[var(--tx)]">{{ txParts.value }}</span>
            <span class="text-xs text-muted">{{ txParts.unit }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="relative mt-4 h-56 flex-1 sm:h-64">
      <Chart
        type="line"
        :data="chartData"
        :options="chartOptions"
        class="h-full w-full"
        aria-label="Live throughput chart"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
const { samples, current } = useLive()
const { colors } = useChart()
const { speedParts, formatSpeed } = useFormat()
const { tz } = useDisplayTz()

const rxParts = computed(() => speedParts(current.value.rx_bps))
const txParts = computed(() => speedParts(current.value.tx_bps))

// The live window only spans a couple of minutes, so the hour and AM/PM made
// labels wide enough to overlap. Show minute:second only (e.g. "51:20"), in the
// user-selected display timezone. formatToParts guarantees the "MM:SS" shape
// regardless of locale.
const timeFmt = computed(
  () =>
    new Intl.DateTimeFormat('en-GB', {
      timeZone: tz.value,
      hourCycle: 'h23',
      minute: '2-digit',
      second: '2-digit',
    }),
)

function fmtTime(ms: number, fmt: Intl.DateTimeFormat): string {
  const parts = fmt.formatToParts(ms)
  const mm = parts.find((p) => p.type === 'minute')?.value ?? '00'
  const ss = parts.find((p) => p.type === 'second')?.value ?? '00'
  return `${mm}:${ss}`
}

const chartData = computed(() => {
  const fmt = timeFmt.value
  return {
    labels: samples.value.map((s) => fmtTime(s.ts, fmt)),
    datasets: [
      {
        label: 'Download',
        data: samples.value.map((s) => s.rx_bps),
        borderColor: colors.rx,
        backgroundColor: colors.rxFill,
        fill: true,
        tension: 0.35,
        pointRadius: 0,
        borderWidth: 2,
      },
      {
        label: 'Upload',
        data: samples.value.map((s) => s.tx_bps),
        borderColor: colors.tx,
        backgroundColor: colors.txFill,
        fill: true,
        tension: 0.35,
        pointRadius: 0,
        borderWidth: 2,
      },
    ],
  }
})

const chartOptions = computed(() => {
  return {
    responsive: true,
    maintainAspectRatio: false,
    animation: false as const,
    interaction: { intersect: false, mode: 'index' as const },
    scales: {
      x: {
        grid: { display: false },
        ticks: { color: colors.ticks, maxTicksLimit: 6, autoSkip: true, maxRotation: 0, font: { family: colors.font } },
      },
      y: {
        beginAtZero: true,
        grid: { color: colors.grid },
        ticks: { color: colors.ticks, font: { family: colors.font }, callback: (v: number) => formatSpeed(Number(v)) },
      },
    },
    plugins: {
      legend: { display: false },
      tooltip: {
        backgroundColor: colors.tooltipBg,
        borderColor: colors.tooltipBorder,
        borderWidth: 1,
        titleColor: colors.tooltipText,
        bodyColor: colors.tooltipText,
        padding: 10,
        callbacks: {
          title: (items: any[]) => {
            const s = samples.value[items[0]?.dataIndex]
            if (!s) return ''
            return new Date(s.ts).toLocaleTimeString('en-GB', {
              timeZone: tz.value,
              hourCycle: 'h23',
              hour: '2-digit',
              minute: '2-digit',
              second: '2-digit',
            })
          },
          label: (ctx: any) => ` ${ctx.dataset.label}: ${formatSpeed(Number(ctx.parsed.y))}`,
        },
      },
    },
  }
})
</script>
