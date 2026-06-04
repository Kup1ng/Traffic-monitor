<template>
  <div class="min-h-dvh grid place-items-center px-4">
    <div class="w-full max-w-sm">
      <div class="mb-8 flex items-center justify-center gap-3">
        <svg viewBox="0 0 24 24" fill="none" stroke="var(--brand)" stroke-width="2.5" stroke-linecap="round"
          stroke-linejoin="round" class="h-7 w-7">
          <path d="M2 12h4l3 8 4-16 3 8h6" />
        </svg>
        <h1 class="text-xl font-semibold tracking-tight text-ink">Traffic Monitor</h1>
      </div>

      <form class="clay p-7" @submit.prevent="onSubmit">
        <label for="pw" class="mb-2 block text-sm text-muted">Admin password</label>
        <Password
          input-id="pw"
          v-model="password"
          :feedback="false"
          toggle-mask
          fluid
          :disabled="loading"
          autofocus
          aria-describedby="pw-error"
        />

        <p
          v-if="error"
          id="pw-error"
          role="alert"
          aria-live="polite"
          class="mt-3 flex items-center gap-2 text-sm text-down"
        >
          <i class="pi pi-exclamation-circle" />
          {{ error }}
        </p>

        <Button type="submit" label="Sign in" class="mt-5 w-full" :loading="loading" />
      </form>

      <p v-if="!passwordConfigured" class="mt-4 text-center text-xs text-muted">
        No password configured yet — run
        <code class="text-ink">traffic-monitor set-password</code> on the server.
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
const password = ref('')
const error = ref('')
const loading = ref(false)
const { login, passwordConfigured } = useAuth()

async function onSubmit() {
  if (loading.value) return
  error.value = ''
  loading.value = true
  try {
    await login(password.value)
    await navigateTo('/')
  } catch (e: any) {
    error.value = e?.data?.error || 'Sign in failed. Check the password and try again.'
  } finally {
    loading.value = false
  }
}
</script>
