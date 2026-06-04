<template>
  <header class="top-0 z-40 bg-[var(--bg)]/70 backdrop-blur md:sticky">
    <div class="mx-auto max-w-7xl px-3 py-3 sm:px-6">
      <div class="clay-sm flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-2.5 sm:px-5">
        <!-- Brand -->
        <div class="flex items-center gap-2.5">
          <svg viewBox="0 0 24 24" fill="none" stroke="var(--brand)" stroke-width="2.5" stroke-linecap="round"
            stroke-linejoin="round" class="h-6 w-6">
            <path d="M2 12h4l3 8 4-16 3 8h6" />
          </svg>
          <span class="text-base font-semibold tracking-tight text-ink">Traffic Monitor</span>
          <span
            v-if="iface?.demo"
            class="rounded-lg bg-warn/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-warn"
          >
            Demo
          </span>
        </div>

        <div class="ml-auto flex flex-wrap items-center gap-x-3 gap-y-2 sm:gap-x-4">
          <!-- Interface -->
          <div class="clay-inset flex items-center gap-2 px-3 py-1.5">
            <span class="h-2 w-2 rounded-full" :class="stateDot" :title="iface?.operstate || 'unknown'" />
            <span class="tnum text-sm text-ink">{{ iface?.name || '—' }}</span>
          </div>

          <!-- Live link status -->
          <span
            class="inline-flex items-center gap-1.5 text-xs font-medium"
            :class="connected ? 'text-ok' : 'text-muted'"
            :title="connected ? 'Live stream connected' : 'Live stream disconnected'"
          >
            <span class="relative flex h-2 w-2">
              <span
                v-if="connected"
                class="absolute inline-flex h-full w-full animate-ping rounded-full bg-ok"
              />
              <span class="relative inline-flex h-2 w-2 rounded-full" :class="connected ? 'bg-ok' : 'bg-muted'" />
            </span>
            <span class="hidden sm:inline">{{ connected ? 'Live' : 'Offline' }}</span>
          </span>

          <!-- Logout -->
          <Button
            icon="pi pi-sign-out"
            label="Sign out"
            severity="secondary"
            size="small"
            @click="onLogout"
          />
        </div>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import type { InterfaceResp } from '~/composables/useApi'

const props = defineProps<{ iface: InterfaceResp | null; connected: boolean }>()
const { logout } = useAuth()

const stateDot = computed(() => {
  switch (props.iface?.operstate) {
    case 'up':
      return 'bg-up'
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
