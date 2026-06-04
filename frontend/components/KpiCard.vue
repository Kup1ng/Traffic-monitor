<template>
  <div
    class="rounded-2xl border border-edge bg-surface/60 p-5 shadow-lg backdrop-blur transition-colors hover:border-white/15"
    :style="{ '--accent': accentColor }"
  >
    <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-muted">{{ label }}</span>
      <span
        class="grid h-9 w-9 place-items-center rounded-xl"
        :style="{ background: 'color-mix(in srgb, var(--accent) 16%, transparent)' }"
      >
        <i :class="icon" :style="{ color: 'var(--accent)' }" />
      </span>
    </div>
    <div class="mt-4 flex items-baseline gap-1.5">
      <span class="tnum text-3xl font-semibold tracking-tight text-ink">{{ parts.value }}</span>
      <span class="text-base font-medium text-muted">{{ parts.unit }}</span>
    </div>
    <p v-if="sub" class="mt-1 text-xs text-muted">{{ sub }}</p>
  </div>
</template>

<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    label: string
    icon: string
    parts: { value: string; unit: string }
    accent?: 'rx' | 'tx' | 'brand'
    sub?: string
  }>(),
  { accent: 'brand', sub: '' },
)

const accentColor = computed(
  () => ({ rx: '#38bdf8', tx: '#fb923c', brand: '#3b82f6' })[props.accent],
)
</script>
