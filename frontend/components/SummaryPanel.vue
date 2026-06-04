<template>
  <div class="flex h-full flex-col clay p-6">
    <h2 class="text-sm font-semibold text-ink">Usage summary</h2>

    <div v-if="!summary" class="flex flex-1 items-center justify-center py-8">
      <i class="pi pi-spin pi-spinner text-muted" />
    </div>

    <ul v-else class="mt-3 flex flex-1 flex-col divide-y divide-black/5">
      <li
        v-for="row in rows"
        :key="row.label"
        class="flex items-center justify-between gap-3 py-3"
        :class="row.strong ? 'mt-auto border-t border-black/10 pt-3' : ''"
      >
        <span class="text-sm" :class="row.strong ? 'font-semibold text-ink' : 'text-muted'">
          {{ row.label }}
        </span>
        <div class="flex items-center gap-4 text-sm">
          <span class="inline-flex items-center gap-1.5" title="Download">
            <i class="pi pi-arrow-down text-[var(--rx)]" style="font-size: 0.7rem" />
            <span class="tnum text-ink">{{ formatBytes(row.p.rx) }}</span>
          </span>
          <span class="inline-flex items-center gap-1.5" title="Upload">
            <i class="pi pi-arrow-up text-[var(--tx)]" style="font-size: 0.7rem" />
            <span class="tnum text-ink">{{ formatBytes(row.p.tx) }}</span>
          </span>
        </div>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import type { SummaryResp } from '~/composables/useApi'

const props = defineProps<{ summary: SummaryResp | null }>()
const { formatBytes } = useFormat()

const rows = computed(() => {
  const s = props.summary
  if (!s) return []
  return [
    { label: 'Today', p: s.today, strong: false },
    { label: 'Last 24 hours', p: s.last_24h, strong: false },
    { label: 'This month', p: s.this_month, strong: false },
    { label: 'All-time', p: s.all_time, strong: true },
  ]
})
</script>
