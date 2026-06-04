<template>
  <header
    class="sticky top-0 z-40 border-b border-edge bg-bg/80 backdrop-blur supports-[backdrop-filter]:bg-bg/60"
  >
    <div class="mx-auto flex max-w-7xl flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 sm:px-6">
      <!-- Brand -->
      <div class="flex items-center gap-2.5">
        <svg viewBox="0 0 24 24" fill="none" stroke="var(--rx)" stroke-width="2.5" stroke-linecap="round"
          stroke-linejoin="round" class="h-6 w-6">
          <path d="M2 12h4l3 8 4-16 3 8h6" />
        </svg>
        <span class="text-base font-semibold tracking-tight">Traffic Monitor</span>
        <span
          v-if="iface?.demo"
          class="rounded-md border border-warn/40 bg-warn/10 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-warn"
        >
          Demo
        </span>
      </div>

      <div class="ml-auto flex flex-wrap items-center gap-x-4 gap-y-2">
        <!-- Interface -->
        <div class="flex items-center gap-2 rounded-lg border border-edge bg-surface/60 px-2.5 py-1.5">
          <span class="h-2 w-2 rounded-full" :class="stateDot" :title="iface?.operstate || 'unknown'" />
          <span class="tnum text-sm text-ink">{{ iface?.name || '—' }}</span>
        </div>

        <!-- Live link status -->
        <span
          class="inline-flex items-center gap-1.5 text-xs"
          :class="connected ? 'text-ok' : 'text-muted'"
          :title="connected ? 'Live stream connected' : 'Live stream disconnected'"
        >
          <span class="relative flex h-2 w-2">
            <span
              v-if="connected"
              class="absolute inline-flex h-full w-full animate-ping rounded-full bg-ok/70"
            />
            <span class="relative inline-flex h-2 w-2 rounded-full" :class="connected ? 'bg-ok' : 'bg-muted'" />
          </span>
          <span class="hidden sm:inline">{{ connected ? 'Live' : 'Offline' }}</span>
        </span>

        <!-- Units toggle -->
        <SelectButton
          v-model="speedUnit"
          :options="unitOptions"
          option-label="label"
          option-value="value"
          :allow-empty="false"
          aria-label="Speed units"
          size="small"
        />

        <!-- Logout -->
        <Button
          icon="pi pi-sign-out"
          label="Sign out"
          severity="secondary"
          text
          size="small"
          @click="onLogout"
        />
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import type { InterfaceResp } from '~/composables/useApi'

const props = defineProps<{ iface: InterfaceResp | null; connected: boolean }>()
const { speedUnit } = useFormat()
const { logout } = useAuth()

const unitOptions = [
  { label: 'bits/s', value: 'bits' },
  { label: 'bytes/s', value: 'bytes' },
]

const stateDot = computed(() => {
  switch (props.iface?.operstate) {
    case 'up':
      return 'bg-ok'
    case 'down':
      return 'bg-down'
    default:
      return 'bg-muted'
  }
})

async function onLogout() {
  await logout()
  await navigateTo('/login')
}
</script>
