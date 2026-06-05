<template>
  <div class="inline-flex items-center gap-1.5" :title="hint">
    <i class="pi pi-gauge text-[11px] opacity-70" aria-hidden="true" />
    <form class="inline-flex items-center gap-1" @submit.prevent="apply">
      <input
        v-model="draft"
        inputmode="numeric"
        :disabled="!supported || busy"
        placeholder="off"
        class="tm-bw-input tnum"
        aria-label="Interface bandwidth limit in Mbps"
        @keydown.esc="resetDraft"
      />
      <span class="text-[10px] opacity-70">Mbps</span>
      <button
        v-if="dirty"
        type="submit"
        class="tm-bw-btn"
        :disabled="busy"
        :title="`Apply ${draftNum} Mbps cap`"
        aria-label="Apply bandwidth limit"
      >
        <i class="pi pi-check text-[10px]" />
      </button>
      <button
        v-else-if="limitMbps > 0"
        type="button"
        class="tm-bw-btn"
        :disabled="busy"
        title="Remove the limit (unshaped)"
        aria-label="Remove bandwidth limit"
        @click="clear"
      >
        <i class="pi pi-times text-[10px]" />
      </button>
    </form>

    <span
      v-if="active"
      class="inline-flex items-center gap-1 font-medium text-ok"
      title="Bandwidth limit is active"
    >
      <span class="h-1.5 w-1.5 rounded-full bg-ok" />
      <span class="hidden sm:inline">limited</span>
    </span>
    <span
      v-else-if="limitMbps > 0"
      class="inline-flex items-center gap-1 font-medium text-warn"
      title="Configured but not currently enforced"
    >
      <span class="h-1.5 w-1.5 rounded-full bg-warn" />
      <span class="hidden sm:inline">inactive</span>
    </span>
    <span v-else-if="!supported" class="text-[10px] italic opacity-60">n/a</span>

    <span v-if="msg" class="text-[10px]" :class="msgErr ? 'text-down' : 'text-ok'">{{ msg }}</span>
  </div>
</template>

<script setup lang="ts">
import type { InterfaceResp, ShapingResp } from '~/composables/useApi'

const props = defineProps<{ iface: InterfaceResp | null }>()
const { getShaping, setShaping } = useApi()

const limitMbps = ref(0)
const active = ref(false)
const supported = ref(false)
const draft = ref('')
const busy = ref(false)
const msg = ref('')
const msgErr = ref(false)

const draftNum = computed(() => {
  const n = parseInt(draft.value, 10)
  return Number.isFinite(n) ? n : NaN
})
// A valid, positive limit that differs from what's currently applied.
const dirty = computed(
  () => supported.value && Number.isFinite(draftNum.value) && draftNum.value > 0 && draftNum.value !== limitMbps.value,
)
const hint = computed(() => {
  if (!supported.value) return 'Bandwidth limiting runs on the Linux host (unavailable in demo)'
  const cap = props.iface?.speed_mbps
  return cap && cap > 0
    ? `Hard throughput cap for ${props.iface?.name} — link rate ${cap} Mbps`
    : `Hard throughput cap for ${props.iface?.name}`
})

function applyState(s: ShapingResp) {
  limitMbps.value = s.limit_mbps
  active.value = s.active
  supported.value = s.supported
  draft.value = s.limit_mbps > 0 ? String(s.limit_mbps) : ''
}
function resetDraft() {
  draft.value = limitMbps.value > 0 ? String(limitMbps.value) : ''
}
function flash(text: string, isErr = false) {
  msg.value = text
  msgErr.value = isErr
  setTimeout(() => {
    if (msg.value === text) msg.value = ''
  }, 2500)
}

async function apply() {
  if (!dirty.value || busy.value) return
  busy.value = true
  try {
    applyState(await setShaping(draftNum.value))
    flash(active.value ? 'applied' : 'saved')
  } catch (e: any) {
    flash(e?.data?.error || 'failed', true)
  } finally {
    busy.value = false
  }
}
async function clear() {
  if (busy.value) return
  busy.value = true
  try {
    applyState(await setShaping(0))
    flash('cleared')
  } catch (e: any) {
    flash(e?.data?.error || 'failed', true)
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  try {
    applyState(await getShaping())
  } catch {
    /* non-critical — leave defaults */
  }
})
</script>
