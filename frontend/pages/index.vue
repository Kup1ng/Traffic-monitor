<template>
  <div class="p-6">
    <h1 class="text-lg font-semibold">Dashboard</h1>
    <p class="mt-2 text-sm text-muted">Coming together…</p>
    <Button label="Log out" class="mt-4" severity="secondary" @click="onLogout" />
  </div>
</template>

<script setup lang="ts">
const { logout } = useAuth()
async function onLogout() {
  await logout()
  await navigateTo('/login')
}
</script>
